package peer_discovery

import (
	"sync"
	"testing"
)

func TestInMemoryConcurrentTransactionsAndNotificationSnapshots(t *testing.T) {
	discovery := NewInMemoryPeerDiscovery()
	modelDone := make(chan map[Peer]bool, 1)
	go func() {
		model := map[Peer]bool{}
		for change := range discovery.NotificationChannel() {
			for _, peer := range change.Removed {
				delete(model, peer)
			}
			for _, peer := range change.Added {
				model[peer] = true
			}
			// This must not wait for a writer that is blocked sending its notification.
			discovery.CurrentHosts()
		}
		modelDone <- model
	}()
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			peer := NewPeer("host", port)
			for update := 0; update < 100; update++ {
				discovery.AtomicOperation([]Peer{peer}, nil)
				discovery.RemovePeer(peer)
			}
		}(uint16(worker))
	}
	wg.Wait()
	close(discovery.NotificationChannel())
	model := <-modelDone
	peers, err := discovery.CurrentHosts()
	if err != nil || len(model) != 0 || len(peers) != 0 {
		t.Fatalf("final snapshots: model=%v peers=%v err=%v", model, peers, err)
	}
}
