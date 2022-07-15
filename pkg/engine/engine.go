package engine

import (
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
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/kenlabs/pando/pkg/types/schema"
	"github.com/libp2p/go-libp2p-core/peer"
	"net/http"
	sc "pandoClient/pkg/schema"
	//"github.com/kenlabs/pando/pkg/types/schema"
	"pandoClient/pkg/util/log"
)

var (
	logger             = log.NewSubsystemLogger()
	dsLatestMetaKey    = datastore.NewKey("sync/meta/latest")
	dsPushedCidListKey = datastore.NewKey("sync/meta/list")
)

// Engine is an implementation of the core reference provider interface.
type Engine struct {
	*options
	lsys       ipld.LinkSystem
	publisher  legs.Publisher
	latestMeta cid.Cid
	pushList   []cid.Cid
}

func New(o ...Option) (*Engine, error) {
	opts, err := newOptions(o...)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		options: opts,
	}
	err = e.initInfo(context.Background())
	if err != nil {
		return nil, err
	}

	e.lsys = e.mkLinkSystem()

	return e, nil
}

func (e *Engine) initInfo(ctx context.Context) error {
	metaCid, err := e.getLatestMetaCid(ctx)
	if err != nil {
		return err
	}
	e.latestMeta = metaCid

	pushedList, err := e.GetPushedList(ctx)
	if err != nil {
		return err
	}
	e.pushList = pushedList

	return nil
}

func (e *Engine) Start(ctx context.Context) error {
	var err error

	e.publisher, err = e.newPublisher()
	if err != nil {
		logger.Errorw("Failed to instantiate legs publisher", "err", err, "kind", e.pubKind)
		return err
	}

	// Initialize publisher with latest Meta CID.
	metaCid, err := e.getLatestMetaCid(ctx)
	if err != nil {
		return fmt.Errorf("could not get latest metadata cid: %w", err)
	}
	if metaCid != cid.Undef {
		if err = e.publisher.SetRoot(ctx, metaCid); err != nil {
			return err
		}
	}

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
			return dtsync.NewPublisherFromExisting(e.pubDT, e.h, e.pubTopicName, e.lsys, dtOpts...)
		}
		ds := dsn.Wrap(e.ds, datastore.NewKey("/legs/dtsync/pub"))
		return dtsync.NewPublisher(e.h, ds, e.lsys, e.pubTopicName, dtOpts...)
	case HttpPublisher:
		return httpsync.NewPublisher(e.pubHttpListenAddr, e.lsys, e.h.ID(), e.key)
	default:
		return nil, fmt.Errorf("unknown publisher kind: %s", e.pubKind)
	}
}

func (e *Engine) getLatestMetaCid(ctx context.Context) (cid.Cid, error) {
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
			return nil, nil
		}
		return nil, err
	}
	var res []cid.Cid
	err = json.Unmarshal(b, &res)
	return res, err
}

func (e *Engine) Publish(ctx context.Context, metadata schema.Metadata) (cid.Cid, error) {
	c, err := e.PublishLocal(ctx, metadata)
	if err != nil {
		logger.Errorw("Failed to store advertisement locally", "err", err)
		return cid.Undef, fmt.Errorf("failed to publish advertisement locally: %w", err)
	}

	// Only announce the advertisement CID if publisher is configured.
	if e.publisher != nil {
		log := logger.With("adCid", c)
		log.Info("Publishing advertisement in pubsub channel")
		err = e.publisher.UpdateRoot(ctx, c)
		if err != nil {
			log.Errorw("Failed to announce advertisement on pubsub channel ", "err", err)
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

func (e *Engine) updateLatestMeta(ctx context.Context, c cid.Cid) error {
	if c == cid.Undef {
		return fmt.Errorf("meta cid can not be nil")
	}
	e.latestMeta = c
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
	var prevLink datamodel.Link
	var link datamodel.Link
	if e.latestMeta.Defined() {
		link = ipld.Link(cidlink.Link{Cid: e.latestMeta})
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
	blockHook := func(_ peer.ID, rcid cid.Cid) {
		syncRes = append(syncRes, rcid)
	}
	sync, err := dtsync.NewSync(e.h, e.ds, e.lsys, blockHook)
	if err != nil {
		return nil, err
	}
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

	syncer := sync.NewSyncer(e.pandoAddrinfo.ID, e.pubTopicName, nil)
	err = syncer.Sync(ctx, syncCid, sel)
	return syncRes, nil
}

type latestSyncResJson struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    struct{ Cid string } `json:"Data"`
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

func (e *Engine) Shutdown() error {
	var errs error
	if e.publisher != nil {
		if err := e.publisher.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error closing leg publisher: %s", err))
		}
	}
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
