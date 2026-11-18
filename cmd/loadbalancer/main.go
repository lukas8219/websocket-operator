package main

import (
	"context"
	"flag"
	"log"
	"lukas8219/websocket-operator/cmd/loadbalancer/server"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/transports/builtin"
	"lukas8219/websocket-operator/internal/utils"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	hostsPort := flag.String("adminPort", "8081", "Peer discovery HTTP port")
	port := flag.String("port", "8080", "Port to listen on")
	// mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", true, "Debug mode")
	flag.Parse()
	userHeader, err := config.UserHeaderFromEnv()
	if err != nil {
		log.Fatalf("routing configuration: %v", err)
	}
	discovery, err := config.DiscoveryFromEnv()
	if err != nil {
		log.Fatalf("discovery configuration: %v", err)
	}
	logger.SetupLogger(*debug)
	client, err := utils.NewKubernetesClient()
	if err != nil {
		log.Fatalf("initialize Kubernetes client: %v", err)
	}
	peerDiscovery := peer_discovery.NewKubernetes(peer_discovery.KubernetesPeerDiscoveryOptions{
		Clientset: client,
		Namespace: discovery.Namespace,
		Service:   discovery.Service,
	})
	resolver := resolver.New(
		peerDiscovery,
		consistent_hashing.NewJumpHash(peerDiscovery),
	)
	go resolver.Init()
	transportConfig, err := builtin.FromEnv(*port)
	if err != nil {
		log.Fatalf("transport configuration: %v", err)
	}
	transport, err := builtin.Open(transportConfig, builtin.Options{Resolver: resolver})
	if err != nil {
		log.Fatalf("transport plugin: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server.StartServer(server.ServerConfig{
		Context: ctx, Endpoint: transport, HostsPort: *hostsPort,
		Resolver:   &resolver,
		Port:       *port,
		UserHeader: userHeader,
	})
}
