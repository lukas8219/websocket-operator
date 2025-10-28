package peer_discovery

type Peer struct {
	hostname string
	port     string
}

type PeerDiscovery interface {
	Initialize() error
	GetCurrentHosts() ([]Peer, error)
}
