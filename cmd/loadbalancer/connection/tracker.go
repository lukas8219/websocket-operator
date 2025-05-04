package connection

import (
	"context"
	"log/slog"
	"net"
	"sync"
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
	SetUpstreamHost(host string)
	SetDownstreamConn(conn net.Conn)
	SwitchUpstreamHost(host string)
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
	mu             sync.RWMutex
}

// Create accessor methods without "Get" prefix (more idiomatic in Go)
func (t *Tracker) User() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.user
}

func (t *Tracker) UpstreamHost() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.upstreamHost
}

func (t *Tracker) DownstreamHost() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.downstreamHost
}

func (t *Tracker) UpstreamConn() net.Conn {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.upstreamConn
}

func (t *Tracker) DownstreamConn() net.Conn {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.downstreamConn
}

func (t *Tracker) UpstreamContext() context.Context {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ctx
}

func (t *Tracker) UpstreamCancelChan() chan int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cancelChan
}

func (t *Tracker) SetUpstreamConn(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.upstreamConn = conn
}

func (t *Tracker) SetUpstreamHost(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.upstreamHost = host
}

func (t *Tracker) SetDownstreamConn(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.downstreamConn = conn
}

func (t *Tracker) SwitchUpstreamHost(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancelFunc()
	t.ctx, t.cancelFunc = context.WithCancel(context.Background())
	t.upstreamHost = host
}

// Logging methods with chaining
func (t *Tracker) Info(message string, args ...any) Logger {
	t.mu.RLock()
	user := t.user
	upstreamHost := t.upstreamHost
	downstreamHost := t.downstreamHost
	t.mu.RUnlock()

	slog.With("user", user).
		With("upstreamHost", upstreamHost).
		With("downstreamHost", downstreamHost).
		With("component", "connection-tracker").
		Info(message, args...)
	return t
}

func (t *Tracker) Error(message string, args ...any) Logger {
	t.mu.RLock()
	user := t.user
	upstreamHost := t.upstreamHost
	downstreamHost := t.downstreamHost
	t.mu.RUnlock()

	slog.With("user", user).
		With("upstreamHost", upstreamHost).
		With("downstreamHost", downstreamHost).
		With("component", "connection-tracker").
		Error(message, args...)
	return t
}

func (t *Tracker) Debug(message string, args ...any) Logger {
	t.mu.RLock()
	user := t.user
	upstreamHost := t.upstreamHost
	downstreamHost := t.downstreamHost
	t.mu.RUnlock()

	slog.With("user", user).
		With("upstreamHost", upstreamHost).
		With("downstreamHost", downstreamHost).
		With("component", "connection-tracker").
		Debug(message, args...)
	return t
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
