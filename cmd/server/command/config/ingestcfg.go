package config

import "time"

type PublisherKind string

const (
	defaultPersistAfterSend               = true
	DTSyncPublisherKind     PublisherKind = "dtsync"
	defaultCheckInterval                  = Duration(time.Minute)
)

type IngestCfg struct {
	// todo: not use temporary
	PersistAfterSend bool

	// check whether pushed data is stored in Pando
	CheckInterval Duration

	// in fact, only datatransfer is used
	PublisherKind PublisherKind
}

func NewIngestCfg() IngestCfg {
	return IngestCfg{
		PersistAfterSend: defaultPersistAfterSend,
		PublisherKind:    DTSyncPublisherKind,
		CheckInterval:    defaultCheckInterval,
	}
}

func (ic *IngestCfg) Validate() error {
	return nil
}

func (ic *IngestCfg) PopulateDefaults() {
	if ic.CheckInterval == 0 {
		ic.CheckInterval = defaultCheckInterval
	}
	if ic.PublisherKind == "" {
		ic.PublisherKind = DTSyncPublisherKind
	}
}
