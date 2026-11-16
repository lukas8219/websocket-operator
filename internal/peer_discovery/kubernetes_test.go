package peer_discovery

import (
	"sync"
	"testing"
	"time"
)

func TestKubernetesDiscoveryNotifiesWhenFinalPeerIsRemoved(t *testing.T) {
	discovery := NewKubernetes(KubernetesPeerDiscoveryOptions{}).(*KubernetesPeerDiscovery)
	discovery.updateHostsArray([]string{"10.0.0.1"}, nil)
	<-discovery.NotificationChannel()
	discovery.updateHostsArray(nil, []string{"10.0.0.1"})
	select {
	case event := <-discovery.NotificationChannel():
		if len(event.Added) != 0 || len(event.Removed) != 1 || event.Removed[0].Hostname() != "10.0.0.1" {
			t.Fatalf("unexpected final removal event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification when the final peer was removed")
	}
	peers, err := discovery.CurrentHosts()
	if err != nil || len(peers) != 0 {
		t.Fatalf("hosts after removal: %v, %v", peers, err)
	}
}

func TestKubernetesDiscoveryConcurrentSnapshots(t *testing.T) {
	discovery := NewKubernetes(KubernetesPeerDiscoveryOptions{}).(*KubernetesPeerDiscovery)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			discovery.updateHostsArray([]string{"10.0.0.1"}, nil)
			discovery.updateHostsArray(nil, []string{"10.0.0.1"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			discovery.CurrentHosts()
		}
	}()
	wg.Wait()
}
