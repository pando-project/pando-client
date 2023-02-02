package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-legs"
	"github.com/filecoin-project/go-legs/dtsync"
	"github.com/filecoin-project/go-legs/httpsync"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dsn "github.com/ipfs/go-datastore/namespace"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/kenlabs/pando/pkg/types/schema"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"net/http"
	"pandoClient/cmd/server/command/config"
	sc "pandoClient/pkg/schema"
	"sync"
	"time"

	//"github.com/kenlabs/pando/pkg/types/schema"
	"pandoClient/pkg/util/log"
)

var (
	logger             = log.NewSubsystemLogger()
	dsLatestMetaKey    = datastore.NewKey("sync/meta/latest")
	dsPushedCidListKey = datastore.NewKey("sync/meta/list")
)

// Engine is an implementation of the core reference client interface.
type Engine struct {
	*options
	lsys         *ipld.LinkSystem
	publisher    legs.Publisher
	subscriber   *legs.Subscriber
	latestMeta   cid.Cid
	latestMutex  sync.Mutex
	pushList     []cid.Cid
	publishMutex sync.Mutex
	cr           *checkRegistry
	closing      chan struct{}
	closeDone    chan struct{}
}

func New(o ...Option) (*Engine, error) {
	opts, err := newOptions(o...)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		options:   opts,
		closing:   make(chan struct{}),
		closeDone: make(chan struct{}),
	}
	e.cr, err = newCheckRegistry(e, opts.ds, e.checkInterval)
	if err != nil {
		return nil, err
	}

	err = e.initInfo(context.Background())
	if err != nil {
		return nil, err
	}

	// custom linksystem
	if opts.lsys != nil {
		e.lsys = opts.lsys
	} else {
		e.lsys = e.mkLinkSystem()
	}

	e.bootstrap()

	return e, nil
}

func (e *Engine) bootstrap() {
	closer, err := config.StartBootStrap(e.h, e.options.bootstrap, e.options.pandoAddrinfo)
	if err != nil {
		logger.Errorf("falied to bootstrap: %v", err)
	}
	go func() {
		<-e.closing
		if closer != nil {
			closer.Close()
		}
	}()

}

func (e *Engine) initInfo(ctx context.Context) error {
	metaCid, err := e.getLatestMetaFromDs(ctx)
	if err != nil {
		return err
	}
	e.setLatestMeta(ctx, metaCid)

	pushedList, err := e.GetPushedList(ctx)
	if err != nil {
		return err
	}
	e.pushList = pushedList

	return nil
}

func (e *Engine) Start(ctx context.Context) error {
	var err error

	// create and get topics from unique PubSub, avoid creating multi pubsubs in subscriber and publisher
	e.GossipPubSub, err = pubsub.NewGossipSub(ctx, e.h)
	if err != nil {
		panic(err)
	}
	// get publisher topic from pubsub by topic name
	e.pubTopic, err = e.GossipPubSub.Join(e.pubTopicName)
	if err != nil {
		panic(err)
	}
	// get consumber topic from pubsub by topic name
	e.subTopic, err = e.GossipPubSub.Join(e.subTopicName)
	if err != nil {
		panic(err)
	}

	e.publisher, err = e.newPublisher()
	if err != nil {
		logger.Errorw("Failed to instantiate legs publisher", "err", err, "kind", e.pubKind)
		return err
	}
	e.subscriber, err = e.newSubscriber()
	if err != nil {
		logger.Errorf("Failed to instantiate legs subscriber, err: %v", err)
		return err
	}

	// Initialize publisher with latest Meta CID.
	metaCid, err := e.getLatestMetaFromDs(ctx)
	if err != nil {
		return fmt.Errorf("could not get latest metadata cid: %w", err)
	}
	if metaCid != cid.Undef {
		if err = e.publisher.SetRoot(ctx, metaCid); err != nil {
			return err
		}
	}

	go e.cr.run()

	return nil
}

func (e *Engine) newPublisher() (legs.Publisher, error) {
	switch e.pubKind {
	case NoPublisher:
		logger.Info("Remote announcements is disabled; all metadatas will only be store locally.")
		return nil, nil
	case DataTransferPublisher:
		dtOpts := []dtsync.Option{
			dtsync.Topic(e.pubTopic),
			dtsync.WithExtraData(e.pubExtraGossipData),
		}

		if e.pubDT != nil {
			return dtsync.NewPublisherFromExisting(e.pubDT, e.h, e.pubTopicName, *e.lsys, dtOpts...)
		}
		ds := dsn.Wrap(e.ds, datastore.NewKey("/legs/dtsync/pub"))
		return dtsync.NewPublisher(e.h, ds, *e.lsys, e.pubTopicName, dtOpts...)
	case HttpPublisher:
		return httpsync.NewPublisher(e.pubHttpListenAddr, *e.lsys, e.h.ID(), e.key)
	default:
		return nil, fmt.Errorf("unknown publisher kind: %s", e.pubKind)
	}
}

func (e *Engine) newSubscriber() (*legs.Subscriber, error) {
	subOptions := []legs.Option{
		legs.Topic(e.subTopic),
	}
	ds := dsn.Wrap(e.ds, datastore.NewKey("/legs/dtsync/sub"))

	sub, err := legs.NewSubscriber(e.h, ds, *e.lsys, e.subTopicName, nil, subOptions...)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (e *Engine) getLatestMetaFromDs(ctx context.Context) (cid.Cid, error) {
	b, err := e.ds.Get(ctx, dsLatestMetaKey)
	if err != nil {
		if err == datastore.ErrNotFound {
			return cid.Undef, nil
		}
		return cid.Undef, err
	}
	_, c, err := cid.CidFromBytes(b)
	return c, err
}

func (e *Engine) GetPushedList(ctx context.Context) ([]cid.Cid, error) {
	b, err := e.ds.Get(ctx, dsPushedCidListKey)
	if err != nil {
		if err == datastore.ErrNotFound {
			return make([]cid.Cid, 0), nil
		}
		return nil, err
	}
	var res []cid.Cid
	err = json.Unmarshal(b, &res)
	return res, err
}

// RePublishLatest re-publishes the latest existing metadata to pubsub.
func (e *Engine) RePublishLatest(ctx context.Context) (cid.Cid, error) {
	e.publishMutex.Lock()
	defer e.publishMutex.Unlock()

	metaCid, err := e.getLatestMetaFromDs(ctx)
	if err != nil {
		return cid.Undef, err
	}
	if metaCid.Equals(cid.Undef) {
		return cid.Undef, fmt.Errorf("no pushed metadata to announce, skip announce")
	}
	logger.Infow("Publishing latest metadata", "cid", metaCid)

	// update but not add to the checklist
	err = e.publisher.UpdateRoot(ctx, metaCid)
	if err != nil {
		return cid.Undef, err
	}

	return metaCid, nil
}

func (e *Engine) RePublishCid(ctx context.Context, c cid.Cid) error {
	// don't publish concurrently, it's not safe
	e.publishMutex.Lock()
	defer e.publishMutex.Unlock()
	// recover the root cid, others may sync by cid.Undef.
	defer e.publisher.SetRoot(ctx, e.getLatestMeta(ctx))

	err := e.publisher.UpdateRoot(ctx, c)
	if err != nil {
		return err
	}
	return nil
}

// Publish todo: be sure that the previous cid is correct if you call this function. With concurrent calling, previous cid may be wrong
func (e *Engine) Publish(ctx context.Context, metadata schema.Metadata) (cid.Cid, error) {
	c, err := e.PublishLocal(ctx, metadata)
	if err != nil {
		logger.Errorw("Failed to store advertisement locally", "err", err)
		return cid.Undef, fmt.Errorf("failed to publish advertisement locally: %w", err)
	}

	// Only announce the meta CID if publisher is configured.
	if e.publisher != nil {
		log := logger.With("metaCid", c)
		log.Info("Publishing metadata in pubsub channel")
		err = e.publisher.UpdateRoot(ctx, c)
		if err != nil {
			log.Errorw("Failed to announce metadata on pubsub channel ", "err", err)
			return cid.Undef, err
		}
		err = e.cr.addCheck(c)
		if err != nil {
			log.Errorf("failed to add cid: %s to check list, err: %v", c.String(), err)
			return cid.Undef, err
		}
	} else {
		logger.Errorw("nil publisher!")
	}
	return c, nil
}

func (e *Engine) PublishLocal(ctx context.Context, adv schema.Metadata) (cid.Cid, error) {

	adNode, err := adv.ToNode()
	if err != nil {
		return cid.Undef, err
	}

	lnk, err := e.lsys.Store(ipld.LinkContext{Ctx: ctx}, schema.LinkProto, adNode)
	if err != nil {
		return cid.Undef, fmt.Errorf("cannot generate advertisement link: %s", err)
	}
	c := lnk.(cidlink.Link).Cid
	log := logger.With("adCid", c)
	log.Info("Stored ad in local link system")

	if err := e.updateLatestMeta(ctx, c); err != nil {
		log.Errorw("Failed to update reference to the latest metadata", "err", err)
		return cid.Undef, fmt.Errorf("failed to update reference to latest metadata: %w", err)
	}
	if err := e.updatePushedList(ctx, append(e.pushList, c)); err != nil {
		log.Errorw("Failed to update pushed cid list", "err", err)
		return cid.Undef, fmt.Errorf("failed to update pushed cid list: %w", err)
	}

	log.Info("Updated latest meta cid and cid list successfully")
	return c, nil
}

func (e *Engine) setLatestMeta(ctx context.Context, c cid.Cid) {
	e.latestMutex.Lock()
	defer e.latestMutex.Unlock()
	e.latestMeta = c
}

func (e *Engine) getLatestMeta(ctx context.Context) cid.Cid {
	e.latestMutex.Lock()
	defer e.latestMutex.Unlock()
	return e.latestMeta
}

func (e *Engine) updateLatestMeta(ctx context.Context, c cid.Cid) error {
	if c == cid.Undef {
		return fmt.Errorf("meta cid can not be nil")
	}
	e.setLatestMeta(ctx, c)
	return e.ds.Put(ctx, dsLatestMetaKey, c.Bytes())
}

func (e *Engine) updatePushedList(ctx context.Context, list []cid.Cid) error {
	if list == nil || len(list) == 0 {
		return fmt.Errorf("nil to update")
	}
	e.pushList = list
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return e.ds.Put(ctx, dsPushedCidListKey, b)
}

func (e *Engine) PublishBytesData(ctx context.Context, data []byte) (cid.Cid, error) {
	e.publishMutex.Lock()
	defer e.publishMutex.Unlock()
	var prevLink datamodel.Link
	var link datamodel.Link
	preCid := e.getLatestMeta(ctx)
	if preCid.Defined() {
		link = ipld.Link(cidlink.Link{Cid: preCid})
		prevLink = link
	} else {
		prevLink = nil
	}

	meta, err := sc.NewMetaWithBytesPayload(data, e.h.ID(), e.key, prevLink)
	if err != nil {
		logger.Errorf("failed to generate Metadata, err: %v", err)
		return cid.Undef, err
	}
	c, err := e.Publish(ctx, *meta)
	if err != nil {
		return cid.Undef, err
	}
	return c, nil

}

func (e *Engine) Sync(ctx context.Context, c string, depth int, endCidStr string) ([]cid.Cid, error) {
	syncCid, err := cid.Decode(c)
	if err != nil {
		return nil, err
	}
	var endCid cid.Cid
	if endCidStr != "" {
		endCid, err = cid.Decode(endCidStr)
		if err != nil {
			return nil, err
		}
	}

	var syncRes []cid.Cid
	blockHook := func(_ peer.ID, rcid cid.Cid, _ legs.SegmentSyncActions) {
		syncRes = append(syncRes, rcid)
	}

	// if sel is nil, sync will raise error
	var sel ipld.Node
	if depth != 0 || endCid.Defined() {
		var limiter selector.RecursionLimit
		var endLink ipld.Link
		if depth != 0 {
			limiter = selector.RecursionLimitDepth(int64(depth))
		}
		if endCid.Defined() {
			endLink = cidlink.Link{Cid: endCid}
		}
		sel = legs.LegSelector(limiter, endLink)
	} else {
		sel = legs.LegSelector(selector.RecursionLimitDepth(999999), nil)
	}

	_, err = e.subscriber.Sync(ctx, e.pandoAddrinfo.ID, syncCid, sel, nil, legs.ScopedBlockHook(blockHook))

	return syncRes, nil
}

type latestSyncResJson struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    struct{ Cid string } `json:"Data"`
}

type inclusionResJson struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    *MetaInclusion `json:"Data"`
}

func (e *Engine) SyncWithProvider(ctx context.Context, provider string, depth int, endCid string) error {
	res, err := handleResError(e.pandoAPIClient.R().Get("/provider/head?peerid=" + provider))
	if err != nil {
		return err
	}
	resJson := latestSyncResJson{}
	err = json.Unmarshal(res.Body(), &resJson)
	if err != nil {
		logger.Errorf("failed to unmarshal the latestes cid from PandoAPI result: %v", err)
		return err
	}

	_, err = e.Sync(ctx, resJson.Data.Cid, depth, endCid)
	if err != nil {
		return err
	}

	return nil
}

func (e *Engine) CatCid(ctx context.Context, c cid.Cid) ([]byte, error) {
	n, err := e.lsys.Load(ipld.LinkContext{Ctx: ctx}, cidlink.Link{Cid: c}, schema.MetadataPrototype)
	if err != nil {
		if err == datastore.ErrNotFound {
			logger.Infof("not found cid: %s locally, try sync from Pando", c.String())
			// todo: the context can not break the sync while timeout, we need a method to break
			cctx, cncl := context.WithTimeout(ctx, time.Second*15)
			defer cncl()
			n, err = e.catRemote(cctx, c)
			if err != nil {
				logger.Errorf("failed to sync cid: %s from Pando, err: %v", c.String(), err)
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	meta, err := schema.UnwrapMetadata(n)
	if err != nil {
		return nil, err
	}
	dataNode := meta.Payload
	bytesRes, err := dataNode.AsBytes()
	// bytes node
	if err == nil {
		return bytesRes, nil
	} else {
		// try dagjson encode
		buf := bytes.Buffer{}
		err = dagjson.Encode(dataNode, &buf)
		if err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

func (e *Engine) catRemote(ctx context.Context, c cid.Cid) (datamodel.Node, error) {
	syncCids, err := e.Sync(ctx, c.String(), 1, "")
	if err != nil {
		return nil, err
	}
	if len(syncCids) != 1 {
		logger.Errorf("sync successfully but got wrong node number: %d, expected: 1", len(syncCids))
		return nil, fmt.Errorf("wrong nodes number")
	}
	if !syncCids[0].Equals(c) {
		logger.Errorf("sync node dismatched the cid, expected: %s, got: %s", c.String(), syncCids[0].String())
		return nil, fmt.Errorf("sync node dismatched cid")
	}
	n, err := e.lsys.Load(ipld.LinkContext{Ctx: ctx}, cidlink.Link{Cid: c}, schema.MetadataPrototype)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (e *Engine) Shutdown() error {
	var errs error
	if e.publisher != nil {
		if err := e.publisher.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error closing leg publisher: %s", err))
		}
	}
	close(e.closing)
	e.cr.close()
	close(e.closeDone)

	<-e.closeDone

	return errs
}

func handleResError(res *resty.Response, err error) (*resty.Response, error) {
	errTmpl := "failed to get latest head, error: %v"
	if err != nil {
		return res, err
	}
	if res.IsError() {
		return res, fmt.Errorf(errTmpl, res.Error())
	}
	if res.StatusCode() != http.StatusOK {
		return res, fmt.Errorf(errTmpl, fmt.Sprintf("expect 200, got %d", res.StatusCode()))
	}

	return res, nil
}
