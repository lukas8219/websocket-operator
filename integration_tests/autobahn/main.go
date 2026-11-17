// Command autobahn runs the production load balancer against one stock echo peer.
package main

import (
	"time"

	"lukas8219/websocket-operator/cmd/loadbalancer/server"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
)

func main() {
	discovery := peer_discovery.NewInMemoryPeerDiscovery()
	routes := resolver.New(discovery, consistent_hashing.NewJumpHash(discovery))
	go routes.Init()
	discovery.AddPeer(peer_discovery.NewPeer("127.0.0.1", 9001))
	for {
		if _, err := routes.Lookup([]byte("ready")); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// Stock Autobahn sends User-Agent but has no custom-header setting.
	server.StartServer(server.ServerConfig{Resolver: &routes, Port: "9000", UserHeader: "User-Agent"})
}
