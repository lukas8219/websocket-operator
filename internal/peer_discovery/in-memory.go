package peer_discovery

import (
	"lukas8219/websocket-operator/internal/diff"
	"sync"

	"github.com/hashicorp/go-set/v3"
)

type InMemoryPeerDiscovery struct {
	// Serialize mutations and their notifications, but release mu before sending
	// so a notification consumer can safely request a snapshot.
	updates        sync.Mutex
	mu             sync.RWMutex
	availablePeers *set.Set[Peer]
	channel        chan diff.DifferenceOutput[Peer]
}

func NewInMemoryPeerDiscovery() *InMemoryPeerDiscovery {
	return &InMemoryPeerDiscovery{
		availablePeers: set.New[Peer](1),
		channel:        make(chan diff.DifferenceOutput[Peer]),
	}
}

func (m *InMemoryPeerDiscovery) Initialize() error { return nil }

func (m *InMemoryPeerDiscovery) CurrentHosts() ([]Peer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.availablePeers.Slice(), nil
}

func (m *InMemoryPeerDiscovery) Mode() PeerDiscoveryMode { return PeerDiscoveryModeInMemory }
func (m *InMemoryPeerDiscovery) NotificationChannel() chan diff.DifferenceOutput[Peer] {
	return m.channel
}

func (m *InMemoryPeerDiscovery) AddPeer(peer Peer) {
	m.updates.Lock()
	defer m.updates.Unlock()
	m.mu.Lock()
	added := m.availablePeers.Insert(peer)
	m.mu.Unlock()
	if added {
		m.channel <- diff.DifferenceOutput[Peer]{Added: []Peer{peer}}
	}
}

func (m *InMemoryPeerDiscovery) RemovePeer(peer Peer) {
	m.updates.Lock()
	defer m.updates.Unlock()
	m.mu.Lock()
	removed := m.availablePeers.Remove(peer)
	m.mu.Unlock()
	if removed {
		m.channel <- diff.DifferenceOutput[Peer]{Removed: []Peer{peer}}
	}
}

func (m *InMemoryPeerDiscovery) AtomicOperation(newPeers, removePeers []Peer) {
	m.updates.Lock()
	defer m.updates.Unlock()
	m.mu.Lock()
	changes := diff.Difference(m.availablePeers, newPeers, removePeers)
	m.mu.Unlock()
	m.channel <- changes
}
