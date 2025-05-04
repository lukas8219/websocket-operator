package connection

import (
	"bufio"
	"context"
	"net"

	"github.com/gobwas/ws"
)

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
