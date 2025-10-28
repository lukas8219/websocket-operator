package peer_discovery

import (
	"fmt"
	"lukas8219/websocket-operator/internal/diff"
)

type Peer struct {
	hostname string
	port     uint16
}

func (p Peer) Hostname() string {
	return p.hostname
}

func (p *Peer) String() string {
	return p.hostname
}

func (p Peer) Port() uint16 {
	return p.port
}

func (p Peer) SocketAddres() string {
	return fmt.Sprintf("%s:%d", p.hostname, p.port)
}

type PeerDiscovery interface {
	Initialize() error
	CurrentHosts() ([]Peer, error)
	NotificationChannel() chan diff.DifferenceOutput[Peer]
	Mode() PeerDiscoveryMode
}

func NewPeer(hostname string, port uint16) Peer {
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
