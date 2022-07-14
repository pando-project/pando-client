package config

type PublisherKind string

const (
	defaultPersistAfterSend               = false
	DTSyncPublisherKind     PublisherKind = "dtsync"
)

type IngestCfg struct {
	PersistAfterSend bool
	PublisherKind    PublisherKind
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
