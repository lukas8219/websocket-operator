package consistent_hashing

import (
	"lukas8219/websocket-operator/internal/peer_discovery"
	"sync"
	"testing"
)

func TestRendezvousTransactionMembershipAndWeights(t *testing.T) {
	r := NewDefault()
	a, b := peer_discovery.NewPeer("a", 3000), peer_discovery.NewPeer("b", 3000)
	r.AddMember(WeightedMember{member: a.String(), weight: 2})
	r.Transaction([]peer_discovery.Peer{a, b}, nil)
	if hosts := r.GetAllHosts(); len(hosts) != 2 {
		t.Fatalf("members = %v, want two", hosts)
	}
	for _, node := range r.GetNodes() {
		if node.member == a.String() && node.weight != 2 {
			t.Fatal("duplicate addition reset existing weight")
		}
	}
	r.Transaction([]peer_discovery.Peer{b}, []peer_discovery.Peer{b})
	if hosts := r.GetAllHosts(); len(hosts) != 1 || hosts[0] != a.String() {
		t.Fatalf("add then remove must leave only a: %v", hosts)
	}
	r.Transaction(nil, []peer_discovery.Peer{a})
	if hosts := r.GetAllHosts(); len(hosts) != 0 {
		t.Fatalf("remaining members: %v", hosts)
	}
}

func TestRendezvousConcurrentTransactionsAndSnapshots(t *testing.T) {
	r := NewDefault()
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			peer := peer_discovery.NewPeer("host", port)
			for i := 0; i < 100; i++ {
				r.Transaction([]peer_discovery.Peer{peer}, nil)
				r.Lookup("recipient")
				r.GetAllHosts()
				r.GetNodes()
				r.Transaction(nil, []peer_discovery.Peer{peer})
			}
		}(uint16(worker))
	}
	wg.Wait()
	if hosts := r.GetAllHosts(); len(hosts) != 0 {
		t.Fatalf("remaining members: %v", hosts)
	}
}
