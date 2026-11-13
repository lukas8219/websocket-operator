package main

import (
	"flag"
	"log"
	"lukas8219/websocket-operator/cmd/loadbalancer/server"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/utils"
)

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	// mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", true, "Debug mode")
	flag.Parse()
	logger.SetupLogger(*debug)
	client, err := utils.NewKubernetesClient()
	if err != nil {
		log.Fatalf("initialize Kubernetes client: %v", err)
	}
	peerDiscovery := peer_discovery.NewKubernetes(peer_discovery.KubernetesPeerDiscoveryOptions{
		Clientset: client,
		Namespace: "default",
		Service:   "ws-proxy-headless",
	})
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
