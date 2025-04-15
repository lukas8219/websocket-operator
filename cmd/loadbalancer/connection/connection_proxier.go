package connection

import (
	"net"
)

type ConnectionProxier interface {
	ProxyUpstreamToDownstream()
	ProxyDownstreamToUpstream() (net.Conn, error)
	ConnectionInfoTracker
}

type ConnectionProxierImpl struct {
	ConnectionInfoTracker
}
