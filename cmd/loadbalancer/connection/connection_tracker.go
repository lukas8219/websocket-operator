package connection

import (
	"context"
	"log/slog"
	"net"
)

// TODO: this interface typing is yet not correct
type ConnectionInfoTracker struct {
	User                   string
	UpstreamHost           string
	DownstreamHost         string
	UpstreamConn           net.Conn
	DownstreamConn         net.Conn
	CancelUpstream         context.CancelFunc
	UpstreamContext        context.Context
	UpstreamCancellingChan chan int
}

type ProxiedConnection interface {
	ConnectionInfoTracker
	ConnectionProxier
}

type ProxiedConnectionImpl struct {
	ConnectionInfoTracker
	ConnectionProxierImpl
}

func (c *ConnectionInfoTracker) Info(message string, args ...any) *ConnectionInfoTracker {
	slog.With("user", c.User).With("upstreamHost", c.UpstreamHost).With("downstreamHost", c.DownstreamHost).With("component", "connection-tracker").Info(message, args...)
	return c
}

func (c *ConnectionInfoTracker) Error(message string, args ...any) *ConnectionInfoTracker {
	slog.With("user", c.User).With("upstreamHost", c.UpstreamHost).With("downstreamHost", c.DownstreamHost).With("component", "connection-tracker").Error(message, args...)
	return c
}

func (c *ConnectionInfoTracker) Debug(message string, args ...any) *ConnectionInfoTracker {
	slog.With("user", c.User).With("upstreamHost", c.UpstreamHost).With("downstreamHost", c.DownstreamHost).With("component", "connection-tracker").Debug(message, args...)
	return c
}

func (c *ConnectionInfoTracker) Close() {
	if c.UpstreamConn != nil {
		c.UpstreamConn.Close()
	}
	if c.DownstreamConn != nil {
		c.DownstreamConn.Close()
	}
}

func NewConnectionProxier(user string, upstreamHost string, downstreamHost string) *ProxiedConnectionImpl {
	c := &ConnectionInfoTracker{
		User:           user,
		UpstreamHost:   upstreamHost,
		DownstreamHost: downstreamHost,
	}
	return &ProxiedConnectionImpl{
		ConnectionInfoTracker: *c,
		ConnectionProxierImpl: ConnectionProxierImpl{
			ConnectionInfoTracker: *c,
		},
	}
}
