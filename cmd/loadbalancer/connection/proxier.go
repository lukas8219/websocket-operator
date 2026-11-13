package connection

import (
	"bufio"
	"context"
	"net"

	"github.com/gobwas/ws"
)

const MAX_RETRIES = 5

// Proxier manages bidirectional proxying of connections
type Proxier interface {
	ProxyUpstreamToDownstream()
	ProxyDownstreamToUpstream() (net.Conn, error)
	Close()
}

type WSDialer interface {
	Dial(ctx context.Context, urlstr string) (net.Conn, *bufio.Reader, ws.Handshake, error)
}

// WSProxier implements Proxier for WebSocket connections
type WSProxier struct {
	tracker *Tracker
	dialer  WSDialer
}

// NewWSProxier creates a new WebSocket proxier
func NewWSProxier(tracker *Tracker, dialer WSDialer) *WSProxier {
	return &WSProxier{
		tracker: tracker,
		dialer:  dialer,
	}
}

func (w *WSProxier) Dial(ctx context.Context, urlstr string) (net.Conn, *bufio.Reader, ws.Handshake, error) {
	return w.dialWithRetry(nil, 0, ctx, urlstr)
}

// TODO remove this. I've added this because while testing, it seems like the Kubernetes reconcile was faster than the API was ready to receive requests
func (w *WSProxier) dialWithRetry(lastError error, retry uint, ctx context.Context, urlstr string) (net.Conn, *bufio.Reader, ws.Handshake, error) {
	if retry == MAX_RETRIES {
		return nil, nil, ws.Handshake{}, lastError
	}
	conn, reader, ws, error := w.dialer.Dial(ctx, urlstr)
	if error != nil {
		return w.dialWithRetry(error, retry+1, ctx, urlstr)
	}
	return conn, reader, ws, nil
}

func (p *WSProxier) Close() {
	upstreamConn := p.tracker.UpstreamConn()
	downstreamConn := p.tracker.DownstreamConn()

	if upstreamConn != nil {
		upstreamConn.Close()
	}
	if downstreamConn != nil {
		downstreamConn.Close()
	}
}
