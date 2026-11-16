package peer_discovery

import (
	"fmt"
	"sync"
	"testing"
)

func TestBehaviorInMemoryConcurrentSnapshots(t *testing.T) {
	d := NewInMemoryPeerDiscovery()
	drained := make(chan struct{})
	go func() {
		for range d.NotificationChannel() {
		}
		close(drained)
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			p := NewPeer(fmt.Sprintf("10.0.0.%d", i%32), 3000)
			d.AddPeer(p)
			d.RemovePeer(p)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 5000; i++ {
			d.CurrentHosts()
		}
	}()
	close(start)
	wg.Wait()
	close(d.NotificationChannel())
	<-drained
}
