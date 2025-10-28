package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/route"
	"net/http"
)

type ServerConfig struct {
	Router route.RouterImpl
	Port   string
}

func StartServer(config ServerConfig) {
	slog.Info("Starting load balancer server", "port", config.Port)
	router := config.Router
	connections := make(map[string]*connection.Connection) //TODO: This could be a broadcast instead of a single recipient/connection
	peerDiscovery := peer_discovery.NewKubernetes("default", "ws-headless-proxy")
	err := peerDiscovery.Initialize()
	if err != nil {
		panic(err) //TODO Better handling
	}
	resolver := resolver.New(peerDiscovery, consistent_hashing.NewJumpHash(peerDiscovery))
	err = resolver.Initialize()
	go handleRebalanceLoop(resolver, connections)
	//TODO how to properly test this - aka not having a server running at all
	http.ListenAndServe("0.0.0.0:"+config.Port, createHandler(router, connections))
}
