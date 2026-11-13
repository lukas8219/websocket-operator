package resolver_test

import (
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"testing"
)

type discoveryStub struct {
	peer_discovery.PeerDiscovery
	calls int
}

func (d *discoveryStub) CurrentHosts() ([]peer_discovery.Peer, error) {
	d.calls++
	return []peer_discovery.Peer{peer_discovery.NewPeer("b", 3000), peer_discovery.NewPeer("a", 3000)}, nil
}

func TestCurrentHostsCachesSortedPeers(t *testing.T) {
	discovery := &discoveryStub{}
	r := resolver.New(discovery, nil)
	for attempt := 0; attempt < 2; attempt++ {
		peers, err := r.CurrentHosts()
		if err != nil {
			t.Fatal(err)
		}
		if len(peers) != 2 || peers[0].Hostname() != "a" || peers[1].Hostname() != "b" {
			t.Fatalf("unexpected sorted peers: %v", peers)
		}
	}
	if discovery.calls != 1 {
		t.Fatalf("discovery calls = %d, want 1", discovery.calls)
	}
}
