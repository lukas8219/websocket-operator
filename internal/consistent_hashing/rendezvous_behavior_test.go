package consistent_hashing

import (
	"lukas8219/websocket-operator/internal/peer_discovery"
	"testing"
	"time"
)

func TestBehaviorRendezvousTransactionCompletes(t *testing.T) {
	r := NewDefault()
	done := make(chan struct{})
	go func() {
		r.Transaction([]peer_discovery.Peer{peer_discovery.NewPeer("127.0.0.1", 3000)}, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Transaction deadlocks when adding its first peer")
	}
}
