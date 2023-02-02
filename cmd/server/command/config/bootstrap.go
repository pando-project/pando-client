package config

import (
	"errors"
	"fmt"
	"github.com/ipfs/go-ipfs/core/bootstrap"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"io"
	"time"
)

const defaultMinimumPeers = 1

// defaultBootstrapAddresses are the hardcoded bootstrap addresses.
var defaultBootstrapAddresses = []string{}

// ErrInvalidPeerAddr signals an address is not a valid peer address.
var ErrInvalidPeerAddr = errors.New("invalid peer address")

type Bootstrap struct {
	// Peers is the local nodes's bootstrap peer addresses
	Peers []string
	// MinimumPeers governs whether to bootstrap more connections. If the node
	// has less open connections than this number, it will open connections to
	// the bootstrap nodes.  Set to 0 to disable bootstrapping.
	MinimumPeers int
}

// NewBootstrap instantiates a new Bootstrap config with default values.
func NewBootstrap() Bootstrap {
	return Bootstrap{
		Peers:        defaultBootstrapPeers(),
		MinimumPeers: defaultMinimumPeers,
	}
}

// PeerAddrs returns the bootstrap peers as a list of AddrInfo.
func (b Bootstrap) PeerAddrs() ([]peer.AddrInfo, error) {
	return parsePeers(b.Peers)
}

// SetPeers sers the bootstrap peers from a list of AddrInfo.
func (b *Bootstrap) SetPeers(addrs []peer.AddrInfo) {
	b.Peers = addrsToPeers(addrs)
}

func (b *Bootstrap) AddPeerWithAddrInfo(addr peer.AddrInfo) {
	peers := addrsToPeers([]peer.AddrInfo{addr})
	b.Peers = append(b.Peers, peers...)
}

// defaultBootstrapPeers returns the (parsed) set of default bootstrap peers.
// Panics on failure as that is a problem with the hardoded addresses.
func defaultBootstrapPeers() []string {
	addrs, err := parsePeers(defaultBootstrapAddresses)
	if err != nil {
		panic(fmt.Sprintf("failed to parse hardcoded bootstrap peers: %s", err))
	}
	return addrsToPeers(addrs)
}

// parsePeers parses a peer list into a list of AddrInfo.
func parsePeers(addrs []string) ([]peer.AddrInfo, error) {
	if len(addrs) == 0 {
		return nil, nil
	}
	maddrs := make([]multiaddr.Multiaddr, len(addrs))
	for i, addr := range addrs {
		var err error
		maddrs[i], err = multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
	}
	return peer.AddrInfosFromP2pAddrs(maddrs...)
}

// addrsToPeers formats a list of AddrInfos as a peer list suitable for
// serialization.
func addrsToPeers(addrs []peer.AddrInfo) []string {
	peers := make([]string, 0, len(addrs))
	for _, pi := range addrs {
		addrs, err := peer.AddrInfoToP2pAddrs(&pi)
		if err != nil {
			// programmer error.
			panic(err)
		}
		for _, addr := range addrs {
			peers = append(peers, addr.String())
		}
	}
	return peers
}

func StartBootStrap(h host.Host, cfg *Bootstrap, pinfo *peer.AddrInfo) (io.Closer, error) {
	// If there are bootstrap peers and bootstrapping is enabled, then try to
	// connect to the minimum set of peers.
	if cfg == nil && pinfo == nil {
		return nil, nil
	}
	if cfg == nil {
		bt := NewBootstrap()
		cfg = &bt
	}
	if cfg.MinimumPeers != 0 || pinfo != nil {
		addrs, err := cfg.PeerAddrs()
		if err != nil {
			return nil, fmt.Errorf("bad bootstrap peer: %s", err)
		}

		addrs = append(addrs, *pinfo)

		bootCfg := bootstrap.BootstrapConfigWithPeers(addrs)
		bootCfg.MinPeerThreshold = cfg.MinimumPeers
		// move to config after
		bootCfg.Period = time.Second * 30
		bootCfg.ConnectionTimeout = time.Second * 5

		peerID := h.ID()
		bootstrapper, err := bootstrap.Bootstrap(peerID, h, nil, bootCfg)
		if err != nil {
			return nil, fmt.Errorf("bootstrap failed: %s", err)
		}
		return bootstrapper, nil
	}
	return nil, nil
}
