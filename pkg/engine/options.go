package engine

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipld/go-ipld-prime/linking"
	"github.com/libp2p/go-libp2p"
	"pandoClient/cmd/server/command/config"
	"time"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	// NoPublisher indicates that no announcements are made to the network and all metadatas
	// are only stored locally.
	NoPublisher PublisherKind = ""

	// DataTransferPublisher makes announcements over a gossipsub topic and exposes a
	// datatransfer/graphsync server that allows peers in the network to sync metadatas.
	DataTransferPublisher PublisherKind = "dtsync"

	// HttpPublisher exposes a HTTP server that announces published metadatas and allows peers
	// in the network to sync them over raw HTTP transport.
	HttpPublisher PublisherKind = "http"
)

type (
	// PublisherKind represents the kind of publisher to use in order to announce a new
	// metadata to the network.
	// See: WithPublisherKind, NoPublisher, DataTransferPublisher, HttpPublisher.
	PublisherKind string

	// Option sets a configuration parameter for the provider engine.
	Option func(*options) error

	options struct {
		ds datastore.Batching
		h  host.Host
		// key is always initialized from the host peerstore.
		// Setting an explicit identity must not be exposed unless it is tightly coupled with the
		// host identity. Otherwise, the signature of metadata will not match the libp2p host
		// ID.
		key crypto.PrivKey

		provider               peer.AddrInfo
		pandoAddrinfo          peer.AddrInfo
		pandoAPIClient         *resty.Client
		checkInterval          time.Duration
		maxIntervalToRepublish time.Duration

		PersistAfterSend bool

		lsys               *linking.LinkSystem
		pubKind            PublisherKind
		pubDT              datatransfer.Manager
		pubHttpListenAddr  string
		GossipPubSub       *pubsub.PubSub
		pubTopicName       string
		pubTopic           *pubsub.Topic
		subTopicName       string
		subTopic           *pubsub.Topic
		pubExtraGossipData []byte
	}
)

func newOptions(o ...Option) (*options, error) {
	opts := &options{
		pubKind:           NoPublisher,
		pubHttpListenAddr: "0.0.0.0:9022",
		// default pando topic, if Pando updates, the topic should update too
		pubTopicName: "/pando/v0.0.1",
		// default topic for consumer, in fact it is not used. But it should not conflict with publisher
		subTopicName:  "pandoClientSubscriberTmp",
		checkInterval: time.Minute,
	}

	for _, apply := range o {
		if err := apply(opts); err != nil {
			return nil, err
		}
	}

	if opts.ds == nil {
		opts.ds = dssync.MutexWrap(datastore.NewMapDatastore())
	}

	if opts.h == nil {
		h, err := libp2p.New()
		if err != nil {
			return nil, err
		}
		logger.Infow("Libp2p host is not configured, but required; created a new host.", "id", h.ID())
		opts.h = h
	}

	// Initialize private key from libp2p host
	opts.key = opts.h.Peerstore().PrivKey(opts.h.ID())
	// Defensively check that host's self private key is indeed set.
	if opts.key == nil {
		return nil, fmt.Errorf("cannot find private key in self peerstore; libp2p host is misconfigured")
	}

	if len(opts.provider.Addrs) == 0 {
		opts.provider.Addrs = opts.h.Addrs()
		logger.Infow("Retrieval address not configured; using host listen addresses instead.", "retrievalAddrs", opts.provider.Addrs)
	}
	if opts.provider.ID == "" {
		opts.provider.ID = opts.h.ID()
		logger.Infow("Retrieval ID not configured; using host ID instead.", "retrievalID", opts.provider.ID)
	}

	return opts, nil
}

func (o *options) retrievalAddrsAsString() []string {
	var ras []string
	for _, ra := range o.provider.Addrs {
		ras = append(ras, ra.String())
	}
	return ras
}

// WithPublisherKind sets the kind of publisher used to announce new metadatas.
// If unset, metadatas are only stored locally and no announcements are made.
// See: PublisherKind.
func WithPublisherKind(k PublisherKind) Option {
	return func(o *options) error {
		o.pubKind = k
		return nil
	}
}

// WithHttpPublisherListenAddr sets the net listen address for the HTTP publisher.
// If unset, the default net listen address of '0.0.0.0:3104' is used.
//
// Note that this option only takes effect if the PublisherKind is set to HttpPublisher.
// See: WithPublisherKind.
func WithHttpPublisherListenAddr(addr string) Option {
	return func(o *options) error {
		o.pubHttpListenAddr = addr
		return nil
	}
}

// WithTopicName sets toe topic name on which pubsub announcements are published.
//
// Note that this option only takes effect if the PublisherKind is set to DataTransferPublisher.
// See: WithPublisherKind.
func WithTopicName(t string) Option {
	return func(o *options) error {
		o.pubTopicName = t
		return nil
	}
}

// WithDataTransfer sets the instance of datatransfer.Manager to use.
// If unspecified a new instance is created automatically.
//
// Note that this option only takes effect if the PublisherKind is set to DataTransferPublisher.
// See: WithPublisherKind.
func WithDataTransfer(dt datatransfer.Manager) Option {
	return func(o *options) error {
		o.pubDT = dt
		return nil
	}
}

// WithHost specifies the host to which the provider engine belongs.
// If unspecified, a host is created automatically.
// See: libp2p.New.
func WithHost(h host.Host) Option {
	return func(o *options) error {
		o.h = h
		return nil
	}
}

// WithDatastore sets the datastore that is used by the engine to store metadatas.
// If unspecified, an ephemeral in-memory datastore is used.
// See: datastore.NewMapDatastore.
func WithDatastore(ds datastore.Batching) Option {
	return func(o *options) error {
		o.ds = ds
		return nil
	}
}

func WithLinkSystem(lsys *linking.LinkSystem) Option {
	return func(o *options) error {
		o.lsys = lsys
		return nil
	}
}

func WithSubTopicName(tname string) Option {
	return func(o *options) error {
		o.subTopicName = tname
		return nil
	}
}

// WithExtraGossipData supplies extra data to include in the pubsub announcement.
// Note that this option only takes effect if the PublisherKind is set to DataTransferPublisher.
// See: WithPublisherKind.
func WithExtraGossipData(extraData []byte) Option {
	return func(o *options) error {
		if len(extraData) != 0 {
			// Make copy for safety.
			o.pubExtraGossipData = make([]byte, len(extraData))
			copy(o.pubExtraGossipData, extraData)
		}
		return nil
	}
}

func WithPandoAddrinfo(addrinfo peer.AddrInfo) Option {
	return func(o *options) error {
		o.pandoAddrinfo = addrinfo
		return nil
	}
}

func WithPandoAPIClient(url string, connectTimeout time.Duration) Option {
	return func(o *options) error {
		httpClient := resty.New().SetBaseURL(url).SetTimeout(connectTimeout).SetDebug(false)
		o.pandoAPIClient = httpClient
		return nil
	}
}

func WithCheckInterval(duration config.Duration) Option {
	return func(o *options) error {
		o.checkInterval = time.Duration(duration)
		return nil
	}
}

func WithMaxIntervalToRepublish(duration config.Duration) Option {
	return func(o *options) error {
		o.maxIntervalToRepublish = time.Duration(duration)
		return nil
	}
}

func WithPersistAfterSend(persistAfterSend bool) Option {
	return func(o *options) error {
		o.PersistAfterSend = persistAfterSend
		return nil
	}
}
