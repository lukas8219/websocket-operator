package connection

import (
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (c *ConnectionProxierImpl) ProxyDownstreamToUpstream() (net.Conn, error) {
	host := c.UpstreamHost
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"ws-user-id": []string{c.User},
		},
	}
	c.Debug("Dialing upstream")
	proxiedConn, _, _, err := dialer.Dial(c.UpstreamContext, "ws://"+host)
	if err != nil {
		c.Error("Failed to dial upstream", "error", err)
		return nil, err
	}
	c.Debug("Connected to upstream")
	c.UpstreamConn = proxiedConn

	waitSignal := func() {
		c.Debug("Closing upstream connection")
		select {
		case c.UpstreamCancellingChan <- 1:
			c.Debug("Successfully signaled cancellation")
		default:
			c.Debug("No one waiting for cancellation signal, skipping")
		}
	}

	//Missing DEFER
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-c.UpstreamContext.Done():
				c.Debug("Upstream context done")
				proxiedConn.Close()
				return
			default:
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(proxiedConn)
				if err != nil {
					c.Error("Failed to read from upstream", "error", err)
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(c.DownstreamConn, op, msg)
				if err != nil {
					c.Error("Failed to write to downstream", "error", err)
					return
				}
				if op == ws.OpClose {
					c.Debug("upstream server closed connection")
					return
				}
			}
		}
	}
	go proxySidecarServerToClient()
	return proxiedConn, nil
}
