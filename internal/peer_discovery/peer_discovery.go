package peer_discovery

import (
	"lukas8219/websocket-operator/internal/diff"
)

type Peer struct {
	hostname string
	port     string
}

func (p Peer) Hostname() string {
	return p.hostname
}

func (p *Peer) String() string {
	return p.hostname
}

func (p Peer) Port() string {
	return p.port
}

type PeerDiscovery interface {
	Initialize() error
	CurrentHosts() ([]Peer, error)
	NotificationChannel() chan diff.DifferenceOutput[Peer]
	Mode() PeerDiscoveryMode
}

func NewPeer(hostname string, port string) Peer {
	return Peer{hostname, port}
}

type PeerDiscoveryMode string

type PeerDiscoveryConfig struct {
	Mode       PeerDiscoveryMode
	ConfigMeta interface{}
}

const (
	PeerDiscoveryModeDns        PeerDiscoveryMode = "dns"
	PeerDiscoveryModeKubernetes PeerDiscoveryMode = "kubernetes"
)
