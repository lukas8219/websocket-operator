package server

import (
	"encoding/json"
	"log/slog"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/resolver"
	"net/http"
)

type ServerConfig struct {
	Resolver   *resolver.Resolver
	Port       string
	UserHeader string
}

func StartServer(options ServerConfig) {
	if options.UserHeader == "" {
		options.UserHeader = config.DefaultUserHeader
	}
	slog.Info("Starting load balancer server", "port", options.Port)
	connections := newConnectionRegistry()

	go handleRebalanceLoop(*options.Resolver, connections)
	//TODO how to properly test this - aka not having a server running at all
	go http.ListenAndServe("0.0.0.0:"+options.Port, createHandler(options.Resolver, connections, options.UserHeader))

	http.ListenAndServe("0.0.0.0:8081", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts, _ := (*options.Resolver).CurrentHosts()
		content, _ := json.Marshal(hosts)
		w.WriteHeader(200)
		w.Write(content)
	}))
}
