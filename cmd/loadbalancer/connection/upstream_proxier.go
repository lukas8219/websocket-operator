package connection

import (
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (c *ConnectionProxierImpl) ProxyUpstreamToDownstream() {
	for {
		select {
		case <-c.UpstreamContext.Done():
			c.Debug("Upstream context done")
			return
		default:
			clientConnection := c.DownstreamConn
			upstreamConnection := c.UpstreamConn
			msg, op, err := wsutil.ReadClientData(clientConnection)
			if err != nil {
				c.Error("Failed to read from downstream", "error", err)
				return
			}
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				c.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				c.Debug("downstream server closed connection")
				return
			}
		}
	}
}
