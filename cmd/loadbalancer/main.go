package main

import (
	"flag"
	"lukas8219/websocket-operator/cmd/loadbalancer/server"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
)

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	// mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", true, "Debug mode")
	flag.Parse()
	logger.SetupLogger(*debug)
	peerDiscovery := peer_discovery.NewKubernetes("default", "ws-proxy-headless")
	resolver := resolver.New(
		peerDiscovery,
		consistent_hashing.NewJumpHash(peerDiscovery),
	)
	go resolver.Init()
	server.StartServer(server.ServerConfig{
		Resolver: &resolver,
		Port:     *port,
	})
}
