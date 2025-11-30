package peer_discovery

import (
	"encoding/json"
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

func (p Peer) String() string {
	return p.SocketAddres()
}

func (p Peer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Hostname string `json:"hostname"`
		Port     uint16 `json:"port"`
	}{
		Hostname: p.Hostname(),
		Port:     p.Port(),
	})
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
	PeerDiscoveryModeInMemory   PeerDiscoveryMode = "in-memory"
)
