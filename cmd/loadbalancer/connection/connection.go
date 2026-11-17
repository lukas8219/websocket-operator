package connection

import (
	"net"

	"github.com/gobwas/ws"
)

// Connection combines tracking and proxying capabilities
type Connection struct {
	*Tracker
	Proxier
}

// NewConnection creates a fully configured connection
func NewConnection(user, upstreamHost, downstreamHost string, downstreamConn net.Conn, userHeader string) *Connection {
	tracker := NewTracker(user, upstreamHost, downstreamHost, downstreamConn)
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			userHeader: []string{user},
		},
	}
	proxier := NewWSProxier(tracker, &dialer)

	return &Connection{
		Tracker: tracker,
		Proxier: proxier,
	}
}

// Handle manages the connection lifecycle
func (c *Connection) Handle() {
	proxiedConn, err := c.ProxyDownstreamToUpstream()
	if err != nil {
		return
	}
	c.Tracker.SetUpstreamConn(proxiedConn)

	c.ProxyUpstreamToDownstream()
}

func (c *Connection) Close() {
	c.Proxier.Close()
}
