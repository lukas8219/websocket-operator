package peer_discovery

import (
	"lukas8219/websocket-operator/internal/diff"

	"github.com/hashicorp/go-set/v3"
)

type InMemoryPeerDiscovery struct {
	availablePeers *set.Set[Peer]
	channel        chan diff.DifferenceOutput[Peer]
}

func NewInMemoryPeerDiscovery() InMemoryPeerDiscovery {
	return InMemoryPeerDiscovery{
		availablePeers: set.New[Peer](1),
		channel:        make(chan diff.DifferenceOutput[Peer], 0),
	}
}

func (m InMemoryPeerDiscovery) Initialize() error {
	return nil
}

func (m InMemoryPeerDiscovery) CurrentHosts() ([]Peer, error) {
	return m.availablePeers.Slice(), nil
}

func (m InMemoryPeerDiscovery) Mode() PeerDiscoveryMode {
	return PeerDiscoveryModeInMemory
}

func (m InMemoryPeerDiscovery) NotificationChannel() chan diff.DifferenceOutput[Peer] {
	return m.channel
}

func (m InMemoryPeerDiscovery) AddPeer(peer Peer) {
	if !m.availablePeers.Insert(peer) {
		return
	}
	m.channel <- diff.DifferenceOutput[Peer]{
		Added: []Peer{peer},
	}
}

func (m InMemoryPeerDiscovery) RemovePeer(peer Peer) {
	if !m.availablePeers.Remove(peer) {
		return
	}
	m.channel <- diff.DifferenceOutput[Peer]{
		Removed: []Peer{peer},
	}
}

func (m InMemoryPeerDiscovery) AtomicOperation(NewPeers, RemovePeers []Peer) {
	diff := diff.Difference(m.availablePeers, NewPeers, RemovePeers)
	m.channel <- diff
}
