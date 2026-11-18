package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/transports/link"
	"net/http"
)

type ServerConfig struct {
	Context    context.Context
	Endpoint   link.Endpoint
	HostsPort  string
	Resolver   *resolver.Resolver
	Port       string
	UserHeader string
}

func StartServer(options ServerConfig) {
	if options.UserHeader == "" {
		options.UserHeader = config.DefaultUserHeader
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.HostsPort == "" {
		options.HostsPort = "8081"
	}
	if options.Endpoint == nil {
		endpoint, err := link.NewDirect(options.UserHeader, nil)
		if err != nil {
			slog.Error("Initialize transport", "error", err)
			return
		}
		options.Endpoint = endpoint
	}
	slog.Info("Starting load balancer server", "port", options.Port)
	connections := newConnectionRegistry()

	ctx, cancel := context.WithCancel(options.Context)
	defer cancel()
	defer func() {
		for _, connection := range connections.snapshot() {
			connection.Close()
		}
	}()
	go handleRebalanceLoop(*options.Resolver, connections, ctx)
	//TODO how to properly test this - aka not having a server running at all
	public := &http.Server{Addr: "0.0.0.0:" + options.Port, Handler: createHandler(options.Resolver, connections, options.UserHeader, options.Endpoint)}

	admin := &http.Server{Addr: "0.0.0.0:" + options.HostsPort, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts, _ := (*options.Resolver).CurrentHosts()
		content, _ := json.Marshal(hosts)
		w.WriteHeader(200)
		w.Write(content)
	})}
	if err := link.Serve(ctx, options.Endpoint, public, admin); err != nil {
		slog.Info("Load balancer stopped", "error", err)
	}
}
