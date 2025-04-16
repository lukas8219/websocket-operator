package connection

import (
	"net"
)

// Connection combines tracking and proxying capabilities
type Connection struct {
	*Tracker
	Proxier
}

// NewConnection creates a fully configured connection
func NewConnection(user, upstreamHost, downstreamHost string, downstreamConn net.Conn) *Connection {
	tracker := NewTracker(user, upstreamHost, downstreamHost, downstreamConn)
	proxier := NewWSProxier(tracker)

	return &Connection{
		Tracker: tracker,
		Proxier: proxier,
	}
}

// Handle manages the connection lifecycle
func (c *Connection) Handle() {
	defer c.Close()

	_, err := c.ProxyDownstreamToUpstream()
	if err != nil {
		return
	}

	c.ProxyUpstreamToDownstream()
}
