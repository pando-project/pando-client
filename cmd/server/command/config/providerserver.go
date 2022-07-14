package config

type P2pServer struct {
	// ListenMultiaddr is the multiaddr string for the node's listen address
	ListenMultiaddr string
	// RetrievalMultiaddrs are the addresses to advertise for data retrieval.
	// Defaults to the provider's libp2p host listen addresses.
	RetrievalMultiaddrs []string
}

// NewP2pServer instantiates a new P2pServer config with default values.
func NewP2pServer() P2pServer {
	return P2pServer{
		ListenMultiaddr: "/ip4/0.0.0.0/tcp/9020",
	}
}

// PopulateDefaults replaces zero-values in the config with default values.
func (c *P2pServer) PopulateDefaults() {
	def := NewP2pServer()

	if c.ListenMultiaddr == "" {
		c.ListenMultiaddr = def.ListenMultiaddr
	}
}
