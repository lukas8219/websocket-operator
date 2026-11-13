package server

import (
	"encoding/json"
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/resolver"
	"net/http"
)

type ServerConfig struct {
	Resolver *resolver.Resolver
	Port     string
}

func StartServer(config ServerConfig) {
	slog.Info("Starting load balancer server", "port", config.Port)
	connections := make(map[string]*connection.Connection) //TODO: This could be a broadcast instead of a single recipient/connection

	go handleRebalanceLoop(*config.Resolver, connections)
	//TODO how to properly test this - aka not having a server running at all
	go http.ListenAndServe("0.0.0.0:"+config.Port, createHandler(config.Resolver, connections))

	http.ListenAndServe("0.0.0.0:8081", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts, _ := (*config.Resolver).CurrentHosts()
		content, _ := json.Marshal(hosts)
		w.WriteHeader(200)
		w.Write(content)
	}))
}
