package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
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
	connections := make(map[string]*connection.ProxiedConnectionImpl) //TODO: This could be a broadcast instead of a single recipient/connection
	go handleRebalanceLoop(router, connections)
	//TODO how to properly test this - aka not having a server running at all
	http.ListenAndServe("0.0.0.0:"+config.Port, createHandler(router, connections))
}
