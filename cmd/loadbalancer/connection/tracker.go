package connection

import (
	"context"
	"log/slog"
	"net"
)

// Logger defines the logging behavior
type Logger interface {
	Info(message string, args ...any) Logger
	Error(message string, args ...any) Logger
	Debug(message string, args ...any) Logger
}

// ConnectionTracker tracks connection details and lifecycle
type ConnectionTracker interface {
	Logger
	Close()
	UpstreamConn() net.Conn
	DownstreamConn() net.Conn
	User() string
	UpstreamHost() string
	DownstreamHost() string
	UpstreamContext() context.Context
	UpstreamCancelChan() chan int
	SetUpstreamConn(conn net.Conn)
}

// Tracker implements ConnectionTracker
type Tracker struct {
	user           string
	upstreamHost   string
	downstreamHost string
	upstreamConn   net.Conn
	downstreamConn net.Conn
	cancelFunc     context.CancelFunc
	ctx            context.Context
	cancelChan     chan int
}

// Create accessor methods without "Get" prefix (more idiomatic in Go)
func (t *Tracker) User() string                     { return t.user }
func (t *Tracker) UpstreamHost() string             { return t.upstreamHost }
func (t *Tracker) DownstreamHost() string           { return t.downstreamHost }
func (t *Tracker) UpstreamConn() net.Conn           { return t.upstreamConn }
func (t *Tracker) DownstreamConn() net.Conn         { return t.downstreamConn }
func (t *Tracker) UpstreamContext() context.Context { return t.ctx }
func (t *Tracker) UpstreamCancelChan() chan int     { return t.cancelChan }
func (t *Tracker) SetUpstreamConn(conn net.Conn)    { t.upstreamConn = conn }

// Logging methods with chaining
func (t *Tracker) Info(message string, args ...any) Logger {
	slog.With("user", t.user).
		With("upstreamHost", t.upstreamHost).
		With("downstreamHost", t.downstreamHost).
		With("component", "connection-tracker").
		Info(message, args...)
	return t
}

func (t *Tracker) Error(message string, args ...any) Logger {
	slog.With("user", t.user).
		With("upstreamHost", t.upstreamHost).
		With("downstreamHost", t.downstreamHost).
		With("component", "connection-tracker").
		Error(message, args...)
	return t
}

func (t *Tracker) Debug(message string, args ...any) Logger {
	slog.With("user", t.user).
		With("upstreamHost", t.upstreamHost).
		With("downstreamHost", t.downstreamHost).
		With("component", "connection-tracker").
		Debug(message, args...)
	return t
}

func (t *Tracker) Close() {
	if t.upstreamConn != nil {
		t.upstreamConn.Close()
	}
	if t.downstreamConn != nil {
		t.downstreamConn.Close()
	}
}

// NewTracker creates a new connection tracker
func NewTracker(user, upstreamHost, downstreamHost string, downstreamConn net.Conn) *Tracker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tracker{
		user:           user,
		upstreamHost:   upstreamHost,
		downstreamHost: downstreamHost,
		downstreamConn: downstreamConn,
		ctx:            ctx,
		cancelFunc:     cancel,
		cancelChan:     make(chan int, 1),
	}
}
