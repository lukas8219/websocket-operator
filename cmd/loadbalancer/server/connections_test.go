package server

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
)

func TestConcurrentConnectionSnapshots(t *testing.T) {
	registry := newConnectionRegistry()
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for n := 0; n < 100; n++ {
				tracker := connection.NewTracker(strconv.Itoa(worker), "host", "client", nil)
				current := &connection.Connection{Tracker: tracker}
				registry.store(current)
				for _, c := range registry.snapshot() {
					c.User()
				}
				registry.remove(current)
			}
		}(worker)
	}
	workers.Wait()
	if len(registry.snapshot()) != 0 {
		t.Fatal("removed connection retained")
	}
}

type unavailableResolver struct {
	resolver.Resolver
	versions chan resolver.ResolverVersion
}

func (r unavailableResolver) VersionUpgradeChannel() chan resolver.ResolverVersion { return r.versions }
func (r unavailableResolver) Lookup([]byte) (peer_discovery.Peer, error) {
	return peer_discovery.Peer{}, errors.New("no peers")
}

func TestNoPeersRejectsNewConnectionsAndClosesExistingOnes(t *testing.T) {
	unavailable := unavailableResolver{versions: make(chan resolver.ResolverVersion, 1)}
	registry := newConnectionRegistry()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("ws-user-id", "user")
	response := httptest.NewRecorder()
	handleConnection(unavailable, registry, response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("HTTP %d, want 503", response.Code)
	}
	downstream, peer := net.Pipe()
	defer downstream.Close()
	defer peer.Close()
	registry.store(connection.NewConnection("user", "old:3000", "client", downstream))
	unavailable.versions <- 1
	close(unavailable.versions)
	done := make(chan struct{})
	go func() { handleRebalanceLoop(unavailable, registry); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rebalance blocked with no peers")
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := peer.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("client was not disconnected: %v", err)
	}
	if len(registry.snapshot()) != 0 {
		t.Fatal("unroutable connection retained")
	}
}
