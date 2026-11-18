package connection

import (
	"context"
	"net"
	"sync"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/transports/link"
)

type sessionProxy struct {
	tracker                    *Tracker
	endpoint                   link.Endpoint
	client                     link.Session
	mu                         sync.RWMutex
	current                    link.Session
	dialing                    context.Context
	ready, done                chan struct{}
	started, available, closed sync.Once
}

func NewSessionConnection(user, host, remote string, client net.Conn, endpoint link.Endpoint) *Connection {
	tracker := NewTracker(user, host, remote, client)
	proxy := &sessionProxy{tracker: tracker, endpoint: endpoint, client: link.WebSocket(client, ws.StateServerSide), ready: make(chan struct{}), done: make(chan struct{})}
	return &Connection{Tracker: tracker, session: proxy}
}
func (c *Connection) Done() <-chan struct{} {
	if c.session == nil {
		return nil
	}
	return c.session.done
}
func (c *Connection) SwitchUpstreamHost(host string) {
	if c.session == nil {
		c.Tracker.SwitchUpstreamHost(host)
		return
	}
	p := c.session
	p.mu.Lock()
	select {
	case <-p.done:
	default:
		p.tracker.SwitchUpstreamHost(host)
	}
	p.mu.Unlock()
	select {
	case p.tracker.UpstreamCancelChan() <- 1:
	default:
	}
}

func (p *sessionProxy) handle() {
	p.started.Do(func() { go p.fromClient() })
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		return
	default:
	}
	ctx, host := p.tracker.UpstreamContext(), p.tracker.UpstreamHost()
	if p.dialing == ctx {
		p.mu.Unlock()
		return
	}
	p.dialing = ctx
	p.mu.Unlock()
	next, err := p.endpoint.Dial(ctx, host, p.tracker.User())
	if err != nil {
		if ctx.Err() == nil {
			p.tracker.Error("Transport session open failed", "error", err)
			p.close()
		}
		return
	}
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		next.Close()
		return
	default:
	}
	if ctx.Err() != nil {
		p.mu.Unlock()
		next.Close()
		return
	}
	previous := p.current
	p.current = next
	p.available.Do(func() { close(p.ready) })
	p.mu.Unlock()
	if previous != nil {
		previous.Close()
	}
	go p.fromSidecar(next)
}

// A single client reader survives rebalancing. Switching waits for in-flight
// writes, then closes the old session; failed writes are never replayed.
func (p *sessionProxy) fromClient() {
	defer p.close()
	select {
	case <-p.done:
		return
	case <-p.ready:
	}
	for {
		body, op, err := p.client.Read()
		if err != nil {
			return
		}
		p.mu.RLock()
		if p.current == nil {
			p.mu.RUnlock()
			return
		}
		err = p.current.Write(op, body)
		p.mu.RUnlock()
		if err != nil {
			return
		}
	}
}
func (p *sessionProxy) fromSidecar(source link.Session) {
	for {
		body, op, err := source.Read()
		p.mu.RLock()
		if p.current != source {
			p.mu.RUnlock()
			return
		}
		if err == nil {
			err = p.client.Write(op, body)
		}
		p.mu.RUnlock()
		if err != nil {
			p.close()
			return
		}
	}
}
func (p *sessionProxy) close() {
	p.closed.Do(func() {
		close(p.done)
		p.client.Close()
		p.mu.Lock()
		p.tracker.mu.Lock()
		p.tracker.cancelFunc()
		p.tracker.mu.Unlock()
		current := p.current
		p.current = nil
		p.mu.Unlock()
		if current != nil {
			current.Close()
		}
	})
}
