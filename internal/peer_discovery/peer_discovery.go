package peer_discovery

type Peer struct {
	hostname string
	port     string
}

func (p *Peer) Hostname() string {
	return p.hostname
}

func (p *Peer) Port() string {
	return p.port
}

type PeerDiscovery interface {
	Initialize() error
	CurrentHosts() ([]Peer, error)
	NotificationChannel() chan []Peer
}

func NewPeer(hostname string, port string) Peer {
	return Peer{hostname, port}
}
