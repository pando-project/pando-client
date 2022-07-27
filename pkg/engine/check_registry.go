package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"sync"
	"time"
)

var (
	dsCheckRegistryKey = datastore.NewKey("sync/meta/checkRegistry")
	dsCheckCidListKey  = datastore.NewKey("/checkMap")
)

type syncStatus struct {
	InPando     bool
	CheckTimes  int
	publishTime time.Time
}

type checkRegistry struct {
	checkMutex         sync.Mutex
	checkMap           map[string]*syncStatus
	ds                 datastore.Batching
	e                  *Engine
	checkInterval      time.Duration
	maxTimeToRepublish int
	closing            chan struct{}
	closeDone          chan struct{}
}

func newCheckRegistry(e *Engine, ds datastore.Batching, checkInterval time.Duration) (*checkRegistry, error) {
	childrenDs := namespace.Wrap(ds, dsCheckRegistryKey)
	cr := &checkRegistry{
		checkMap:      make(map[string]*syncStatus),
		e:             e,
		ds:            childrenDs,
		checkInterval: checkInterval,
		closing:       make(chan struct{}),
		closeDone:     make(chan struct{}),
	}
	var err error
	cr.checkMap, err = cr.loadCheckListFromDs(context.Background())
	if err != nil {
		return nil, err
	}

	return cr, nil
}

func (cr *checkRegistry) run() {
	tickerCh := time.NewTicker(cr.checkInterval).C
	for {
		select {
		case _ = <-cr.closing:
			logger.Infow("quit gracefully...")
			close(cr.closeDone)
			return
		case _ = <-tickerCh:
			// copy check map
			_checkMap := make(map[string]*syncStatus)
			cr.checkMutex.Lock()
			if len(cr.checkMap) == 0 {
				cr.checkMutex.Unlock()
				continue
			}
			for c, s := range cr.checkMap {
				// copy the ptr
				_checkMap[c] = s
			}
			cr.checkMutex.Unlock()
			_ = cr.checkSyncStatuses(_checkMap)
			// todo: checkMap will lose if process is shutdown before first check
			err := cr.persistCheckList(context.Background())
			if err != nil {
				logger.Errorf("failed to persist check list, err: %v", err)
			}
		}
	}
}

func (cr *checkRegistry) addCheck(c cid.Cid) error {
	cr.checkMutex.Lock()
	defer cr.checkMutex.Unlock()
	if _, exist := cr.checkMap[c.String()]; exist {
		return fmt.Errorf("has existed in check map")
	}
	cr.checkMap[c.String()] = &syncStatus{
		publishTime: time.Now(),
	}
	return nil
}

func (cr *checkRegistry) checkSyncStatuses(m map[string]*syncStatus) error {
	for cidStr, status := range m {
		select {
		case _ = <-cr.closing:
			close(cr.closeDone)
			return nil
		default:
		}

		c, err := cid.Decode(cidStr)
		if err != nil {
			logger.Errorf("invalid cid in checkmap, delete it. err: %v", err)
			cr.checkMutex.Lock()
			delete(cr.checkMap, cidStr)
			cr.checkMutex.Unlock()
			continue
		}
		err = cr.checkSyncStatus(c, status)
		if err != nil {
			logger.Errorf("failed to check sync status for cid: %s, err: %v", cidStr, err)
		}
	}

	return nil
}

func (cr *checkRegistry) checkSyncStatus(c cid.Cid, status *syncStatus) error {

	res, err := handleResError(cr.e.pandoAPIClient.R().Get("/metadata/inclusion?cid=" + c.String()))
	if err != nil {
		logger.Errorf("failed to check status in Pando for cid: %s, err: %v", c, err)
		return fmt.Errorf("failed to check status in Pando for cid: %s, err: %v", c, err)
	}
	resJson := inclusionResJson{}
	err = json.Unmarshal(res.Body(), &resJson)
	if err != nil {
		logger.Errorf("failed to unmarshal the metaInclusion from PandoAPI result: %v", err)
		return fmt.Errorf("failed to unmarshal the metaInclusion from PandoAPI result: %v", err)
	}
	inclusion := resJson.Data
	if inclusion == nil {
		logger.Errorf("got http response but unexpected inclusion data: %v", resJson.Data)
		return fmt.Errorf("got http response but unexpected inclusion data: %v", resJson.Data)
	}
	// if data is stored in Pando, delete it from checkList
	// todo: if a cid is not stored in Pando after some times check, republish it
	if inclusion.InPando {
		cr.checkMutex.Lock()
		delete(cr.checkMap, c.String())
		if !cr.e.options.PersistAfterSend {
			err := cr.e.ds.Delete(context.Background(), datastore.NewKey(c.String()))
			if err != nil {
				return err
			}
		}
		cr.checkMutex.Unlock()
	} else {
		// option in the copied ptr
		status.CheckTimes++
		// republish if arrived max check times or max interval
		if status.CheckTimes >= cr.maxTimeToRepublish || time.Now().Sub(status.publishTime) > cr.e.options.maxIntervalToRepublish {
			logger.Infow("updated cid not stored in Pando, republish it....", "cid: ", c.String())
			err = cr.e.RePublishCid(context.Background(), c)
			if err != nil {
				logger.Errorf("failed to re-publish cid: %s, err: %v", c.String(), err)
			}
			status.CheckTimes = 0
			status.publishTime = time.Now()
		}
	}

	return nil
}

func (cr *checkRegistry) persistCheckList(ctx context.Context) error {
	cr.checkMutex.Lock()
	defer cr.checkMutex.Unlock()
	if cr.checkMap == nil || len(cr.checkMap) == 0 {
		logger.Info("nil checkMap to persist")
		return nil
	}
	b, err := json.Marshal(cr.checkMap)
	if err != nil {
		return err
	}
	return cr.ds.Put(ctx, dsCheckCidListKey, b)
}

func (cr *checkRegistry) loadCheckListFromDs(ctx context.Context) (map[string]*syncStatus, error) {
	b, err := cr.ds.Get(ctx, dsCheckCidListKey)
	if err != nil {
		if err == datastore.ErrNotFound {
			return make(map[string]*syncStatus), nil
		}
		return nil, err
	}
	var res map[string]*syncStatus
	err = json.Unmarshal(b, &res)
	if err != nil {
		return nil, err
	}

	return res, err
}

func (cr *checkRegistry) close() {
	close(cr.closing)
	<-cr.closeDone
}
