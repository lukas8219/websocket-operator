package server

import (
	"context"
	"log/slog"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"
)

type ConnectionTracker struct {
	user                   string
	upstreamHost           string
	downstreamHost         string
	upstreamConn           net.Conn
	downstreamConn         net.Conn
	cancelUpstream         context.CancelFunc
	upstreamContext        context.Context
	upstreamCancellingChan chan int
}

func (c *ConnectionTracker) Info(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Info(message, args...)
	return c
}

func (c *ConnectionTracker) Error(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Error(message, args...)
	return c
}

func (c *ConnectionTracker) Debug(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Debug(message, args...)
	return c
}

func (c *ConnectionTracker) Close() {
	if c.upstreamConn != nil {
		c.upstreamConn.Close()
	}
	if c.downstreamConn != nil {
		c.downstreamConn.Close()
	}
}

type ServerConfig struct {
	Router route.RouterImpl
	Port   string
}

func StartServer(config ServerConfig) {
	slog.Info("Starting load balancer server", "port", config.Port)
	router := config.Router
	connections := make(map[string]*ConnectionTracker) //TODO: This could be a broadcast instead of a single recipient/connection
	go handleRebalanceLoop(router, connections)
	//TODO how to properly test this - aka not having a server running at all
	http.ListenAndServe("0.0.0.0:"+config.Port, createHandler(router, connections))
}
