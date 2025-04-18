package connection

import (
	"net"
)

// Proxier manages bidirectional proxying of connections
type Proxier interface {
	ProxyUpstreamToDownstream()
	ProxyDownstreamToUpstream() (net.Conn, error)
}

// WSProxier implements Proxier for WebSocket connections
type WSProxier struct {
	tracker ConnectionTracker
}

// NewWSProxier creates a new WebSocket proxier
func NewWSProxier(tracker ConnectionTracker) *WSProxier {
	return &WSProxier{
		tracker: tracker,
	}
}
