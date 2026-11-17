package resolver

import (
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/diff"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"sync"
	"testing"
	"time"
)

type changingDiscovery struct {
	mu     sync.Mutex
	peers  []peer_discovery.Peer
	events chan diff.DifferenceOutput[peer_discovery.Peer]
}

func (d *changingDiscovery) Initialize() error { return nil }
func (d *changingDiscovery) Mode() peer_discovery.PeerDiscoveryMode {
	return peer_discovery.PeerDiscoveryModeInMemory
}
func (d *changingDiscovery) NotificationChannel() chan diff.DifferenceOutput[peer_discovery.Peer] {
	return d.events
}
func (d *changingDiscovery) CurrentHosts() ([]peer_discovery.Peer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]peer_discovery.Peer(nil), d.peers...), nil
}
func (d *changingDiscovery) set(peer peer_discovery.Peer) {
	d.mu.Lock()
	old := d.peers
	d.peers = []peer_discovery.Peer{peer}
	d.mu.Unlock()
	d.events <- diff.DifferenceOutput[peer_discovery.Peer]{Added: []peer_discovery.Peer{peer}, Removed: old}
}

func TestBehaviorResolverUpdatesWithoutVersionSubscriber(t *testing.T) {
	d := &changingDiscovery{events: make(chan diff.DifferenceOutput[peer_discovery.Peer], 4)}
	r := New(d, consistent_hashing.NewJumpHash(d)).(*ResolverImpl)
	done := make(chan struct{})
	go func() { r.Init(); close(done) }()
	// Drain only during cleanup, so even the buggy resolver can exit.
	t.Cleanup(func() {
		close(d.events)
		for {
			select {
			case <-r.VersionUpgradeChannel():
			case <-done:
				return
			case <-time.After(time.Second):
				t.Error("resolver cleanup timed out")
				return
			}
		}
	})
	a := peer_discovery.NewPeer("10.0.0.1", 3000)
	b := peer_discovery.NewPeer("10.0.0.2", 3000)
	d.set(a)
	deadline := time.Now().Add(time.Second)
	for r.version.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	first, err := r.CurrentHosts()
	if err != nil || len(first) != 1 || first[0] != a {
		t.Fatalf("first snapshot: %v %v", first, err)
	}
	d.set(b)
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		peers, err := r.CurrentHosts()
		if err == nil && len(peers) == 1 && peers[0] == b {
			return
		}
		time.Sleep(time.Millisecond)
	}
	peers, _ := r.CurrentHosts()
	t.Fatalf("resolver stopped processing updates without a VersionUpgradeChannel subscriber: got %v, want %v", peers, b)
}

func TestResolverCoalescesPendingVersions(t *testing.T) {
	const updates = 100
	d := &changingDiscovery{events: make(chan diff.DifferenceOutput[peer_discovery.Peer], updates)}
	for i := 0; i < updates; i++ {
		d.set(peer_discovery.NewPeer("host", uint16(i)))
	}
	close(d.events)
	r := New(d, consistent_hashing.NewJumpHash(d)).(*ResolverImpl)
	done := make(chan struct{})
	go func() { r.Init(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber blocked discovery")
	}
	select {
	case version := <-r.VersionUpgradeChannel():
		if version != updates {
			t.Fatalf("pending version = %d, want %d", version, updates)
		}
	default:
		t.Fatal("latest change notification was lost")
	}
	select {
	case version := <-r.VersionUpgradeChannel():
		t.Fatalf("stale notification %d remained", version)
	default:
	}
}
