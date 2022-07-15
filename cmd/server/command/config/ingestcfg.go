package config

type PublisherKind string

const (
	defaultPersistAfterSend               = false
	DTSyncPublisherKind     PublisherKind = "dtsync"
)

type IngestCfg struct {
	// todo: not use temporary
	PersistAfterSend bool
	// in fact, only datatransfer is used
	PublisherKind PublisherKind
}

func NewIngestCfg() IngestCfg {
	return IngestCfg{
		PersistAfterSend: defaultPersistAfterSend,
		PublisherKind:    DTSyncPublisherKind,
	}
}

func (ic *IngestCfg) Validate() error {
	return nil
}
