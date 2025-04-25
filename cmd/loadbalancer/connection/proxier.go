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
}

type WSDialer interface {
	Dial(ctx context.Context, urlstr string) (net.Conn, *bufio.Reader, ws.Handshake, error)
}

// WSProxier implements Proxier for WebSocket connections
type WSProxier struct {
	tracker ConnectionTracker
	dialer  WSDialer
}

// NewWSProxier creates a new WebSocket proxier
func NewWSProxier(tracker ConnectionTracker, dialer WSDialer) *WSProxier {
	return &WSProxier{
		tracker: tracker,
		dialer:  dialer,
	}
}
