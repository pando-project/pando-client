package engine

import (
	"bytes"
	"context"
	"github.com/agiledragon/gomonkey/v2"
	"github.com/filecoin-project/go-legs"
	"github.com/filecoin-project/go-legs/dtsync"
	"github.com/filecoin-project/go-legs/p2p/protocol/head"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	leveldb "github.com/ipfs/go-ds-leveldb"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/storage/memstore"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	selectorbuilder "github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/kenlabs/pando/pkg/types/schema"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	. "github.com/smartystreets/goconvey/convey"
	"pandoClient/cmd/server/command/config"
	"sort"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"testing"
	"time"
)

func contextWithTimeout(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func multiAddsToString(addrs []multiaddr.Multiaddr) []string {
	var rAddrs []string
	for _, addr := range addrs {
		rAddrs = append(rAddrs, addr.String())
	}
	return rAddrs
}

func requireTrueEventually(t *testing.T, attempt func() bool, interval time.Duration, timeout time.Duration, msgAndArgs ...interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if attempt() {
			return
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out awaiting eventual success", msgAndArgs...)
			return
		case <-ticker.C:
		}
	}
}

func requireEqualLegsMessage(t *testing.T, got, want dtsync.Message) {
	require.Equal(t, want.Cid, got.Cid)
	require.Equal(t, want.ExtraData, got.ExtraData)
	wantAddrs, err := want.GetAddrs()
	require.NoError(t, err)
	gotAddrs, err := got.GetAddrs()
	require.NoError(t, err)
	wantAddrsStr := multiAddsToString(wantAddrs)
	sort.Strings(wantAddrsStr)
	gotAddrsStr := multiAddsToString(gotAddrs)
	sort.Strings(gotAddrsStr)
	require.Equal(t, wantAddrsStr, gotAddrsStr)
}

func TestMemEngineCreate(t *testing.T) {
	e, err := New()
	assert.NoError(t, err)
	err = e.Start(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, e.latestMeta, cid.Undef)
	assert.Equal(t, e.pushList, []cid.Cid{})
}
func TestMemEngineShutdown(t *testing.T) {
	e, err := New()
	assert.NoError(t, err)
	err = e.Start(context.Background())
	assert.NoError(t, err)
	err = e.Shutdown()
	assert.NoError(t, err)
}

func TestEngineUpdateInfo(t *testing.T) {
	e, err := New(
		WithPublisherKind(DataTransferPublisher),
	)
	assert.NoError(t, err)
	ctx := context.Background()
	err = e.Start(ctx)
	assert.NoError(t, err)
	cid1, err := e.PublishBytesData(ctx, []byte("123"))
	assert.NoError(t, err)

	cc1, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc1.Equals(cid1))

	cid2, err := e.PublishBytesData(ctx, []byte("abc"))
	assert.NoError(t, err)

	cc2, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc2.Equals(cid2))

	cid3, err := e.PublishBytesData(ctx, []byte("123dsa"))
	assert.NoError(t, err)

	cc3, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc3.Equals(cid3))

	assert.Contains(t, e.pushList, cid1)
	assert.Contains(t, e.pushList, cid2)
	assert.Contains(t, e.pushList, cid3)
}

func TestEngine_PublishWithDataTransferPublisher(t *testing.T) {
	//logging.SetLogLevel("pubsub", "debug")

	ctx := contextWithTimeout(t)
	extraGossipData := []byte("🌧️")
	topic := "123mutouren"

	subHost, err := libp2p.New()
	require.NoError(t, err)

	pubHost, err := libp2p.New()
	require.NoError(t, err)

	subject, err := New(
		WithHost(pubHost),
		WithPublisherKind(DataTransferPublisher),
		WithTopicName(topic),
		WithExtraGossipData(extraGossipData),
	)
	require.NoError(t, err)
	err = subject.Start(ctx)
	require.NoError(t, err)
	defer subject.Shutdown()

	subG, err := pubsub.NewGossipSub(ctx, subHost)
	require.NoError(t, err)

	subT, err := subG.Join(topic)
	require.NoError(t, err)

	subsc, err := subT.Subscribe()
	require.NoError(t, err)

	err = pubHost.Connect(ctx, subHost.Peerstore().PeerInfo(subHost.ID()))
	require.NoError(t, err)

	err = subHost.Connect(ctx, pubHost.Peerstore().PeerInfo(pubHost.ID()))
	require.NoError(t, err)

	//time.Sleep(time.Second * 3)
	requireTrueEventually(t, func() bool {
		pubPeers := subject.GossipPubSub.ListPeers(topic)
		return len(pubPeers) == 1 && pubPeers[0] == subHost.ID()
	}, time.Second, 5*time.Second, "timed out waiting for subscriber peer ID to appear in publisher's gossipsub peer list")

	cid1, err := subject.PublishBytesData(ctx, []byte("testdata1"))
	assert.NoError(t, err)

	gotLatestMetaCid := subject.getLatestMeta(ctx)
	assert.Equal(t, cid1, gotLatestMetaCid)
	has, err := subject.ds.Has(ctx, datastore.NewKey(cid1.String()))
	assert.NoError(t, err)
	assert.True(t, has)

	pubsubMsg, err := subsc.Next(ctx)
	assert.NoError(t, err)
	assert.Equal(t, pubsubMsg.GetFrom(), pubHost.ID())
	assert.Equal(t, pubsubMsg.GetTopic(), topic)

	wantMessage := dtsync.Message{
		Cid:       cid1,
		ExtraData: extraGossipData,
	}
	wantMessage.SetAddrs(subject.h.Addrs())

	gotMessage := dtsync.Message{}
	err = gotMessage.UnmarshalCBOR(bytes.NewBuffer(pubsubMsg.Data))
	assert.NoError(t, err)
	requireEqualLegsMessage(t, wantMessage, gotMessage)

	gotRootCid, err := head.QueryRootCid(ctx, subHost, topic, pubHost.ID())
	require.NoError(t, err)
	require.Equal(t, cid1, gotRootCid)

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	ls := cidlink.DefaultLinkSystem()
	store := &memstore.Store{}
	ls.SetReadStorage(store)
	ls.SetWriteStorage(store)

	sync, err := dtsync.NewSync(subHost, ds, ls, nil)
	require.NoError(t, err)
	syncer := sync.NewSyncer(subject.h.ID(), topic, rate.NewLimiter(100, 10))
	gotHead, err := syncer.GetHead(ctx)
	require.NoError(t, err)
	require.Equal(t, gotLatestMetaCid, gotHead)

	ssb := selectorbuilder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	adSel := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreFields(
		func(efsb selectorbuilder.ExploreFieldsSpecBuilder) {
			efsb.Insert("PreviousID", ssb.ExploreRecursiveEdge())
			efsb.Insert("Next", ssb.ExploreRecursiveEdge())
			efsb.Insert("Entries", ssb.ExploreRecursiveEdge())
		})).Node()
	err = syncer.Sync(ctx, cid1, adSel)

	require.NoError(t, err)
	_, err = store.Get(ctx, cid1.KeyString())
	require.NoError(t, err)
}

func TestMetaInclusion(t *testing.T) {
	t.SkipNow()
	e, err := New(
		WithPublisherKind(DataTransferPublisher),
		WithPandoAPIClient("https://pando-api.kencloud.com", time.Second*10),
	)
	assert.NoError(t, err)
	res, err := e.pandoAPIClient.R().Get("/metadata/inclusion?cid=baguqeeqqw4xw7ahikwhdae2gz4wwiiaxre")
	assert.NoError(t, err)
	t.Log(string(res.Body()))
}

func TestGetPushedList(t *testing.T) {
	Convey("Test Publish and GetPushedList", t, func() {
		ds, err := leveldb.NewDatastore("", nil)
		So(err, ShouldBeNil)

		e, err := New(
			WithDatastore(ds),
			WithPublisherKind(DataTransferPublisher),
		)
		So(err, ShouldBeNil)

		mt, err := schema.NewMetaWithBytesPayload([]byte("hello"), e.h.ID(), e.key)
		So(err, ShouldBeNil)

		ctx := context.Background()
		err = e.Start(ctx)
		So(err, ShouldBeNil)

		c, err := e.Publish(ctx, *mt)
		So(err, ShouldBeNil)
		So(c, ShouldNotBeNil)

		list, err := e.GetPushedList(ctx)
		So(err, ShouldBeNil)
		So(list, ShouldNotBeNil)
		So(list[0], ShouldResemble, c)
	})
}

func TestCatCid(t *testing.T) {
	Convey("Test CatCid", t, func() {
		ctx := context.Background()
		e, err := New(
			WithPublisherKind(DataTransferPublisher),
		)
		So(err, ShouldBeNil)

		err = e.Start(ctx)
		So(err, ShouldBeNil)

		c, err := e.PublishBytesData(ctx, []byte("abc"))
		So(err, ShouldBeNil)

		data, err := e.CatCid(ctx, c)
		So(err, ShouldBeNil)
		So(data, ShouldResemble, []byte("abc"))
	})
}

func TestRePublish(t *testing.T) {
	Convey("Test RePublish", t, func() {
		pandoAddrInfo, err := config.GetPandoInfo()
		So(err, ShouldBeNil)

		info, err := pandoAddrInfo.AddrInfo()
		So(err, ShouldBeNil)

		Convey("RePublishLatest", func() {
			ctx := context.Background()
			e, err := New(
				WithPublisherKind(DataTransferPublisher),
				WithPandoAddrinfo(*info),
			)
			So(err, ShouldBeNil)

			err = e.Start(ctx)
			So(err, ShouldBeNil)

			c, err := e.PublishBytesData(ctx, []byte("abc"))
			So(err, ShouldBeNil)

			rc, err := e.RePublishLatest(ctx)
			So(err, ShouldBeNil)
			So(rc, ShouldResemble, c)

			time.Sleep(time.Second * 3)
		})
		Convey("RePublishCid", func() {
			ctx := context.Background()
			e, err := New(
				WithPublisherKind(DataTransferPublisher),
				WithPandoAddrinfo(*info),
			)
			So(err, ShouldBeNil)

			err = e.Start(ctx)
			So(err, ShouldBeNil)

			c1, err := e.PublishBytesData(ctx, []byte("abc"))
			So(err, ShouldBeNil)
			c2, err := e.PublishBytesData(ctx, []byte("xyz"))
			So(err, ShouldBeNil)

			Latest, err := e.RePublishLatest(ctx)
			So(err, ShouldBeNil)
			So(Latest, ShouldResemble, c2)

			err = e.RePublishCid(ctx, c1)
			So(err, ShouldBeNil)

			time.Sleep(time.Second * 3)

		})
	})
}

func TestSyncFromPando(t *testing.T) {
	t.SkipNow()
	Convey("Test Sync", t, func() {
		pandoAddrInfo, err := config.GetPandoInfo()
		So(err, ShouldBeNil)

		info, err := pandoAddrInfo.AddrInfo()
		So(err, ShouldBeNil)

		ctx := context.Background()
		e, err := New(
			WithPublisherKind(DataTransferPublisher),
			WithPandoAddrinfo(*info),
		)
		So(err, ShouldBeNil)

		err = e.Start(ctx)
		So(err, ShouldBeNil)

		c, err := e.PublishBytesData(ctx, []byte("abc"))
		So(err, ShouldBeNil)

		patch := gomonkey.ApplyMethod(e.subscriber, "Sync", func(_ *legs.Subscriber, ctx context.Context, peerID peer.ID, nextCid cid.Cid, sel ipld.Node, peerAddr multiaddr.Multiaddr, opts ...legs.SyncOption) (cid.Cid, error) {
			return cid.Cid{}, nil
		})
		defer patch.Reset()
		_, err = e.Sync(ctx, c.String(), 0, "")
		So(err, ShouldBeNil)
	})
}
