package connection

import (
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (p *WSProxier) ProxyUpstreamToDownstream() {
	for {
		select {
		case <-p.tracker.UpstreamContext().Done():
			p.tracker.Debug("Upstream context done")
			return
		default:
			clientConnection := p.tracker.DownstreamConn()
			upstreamConnection := p.tracker.UpstreamConn()
			msg, op, err := wsutil.ReadClientData(clientConnection)
			if err != nil {
				p.tracker.Error("Failed to read from downstream", "error", err)
				return
			}
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				p.tracker.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				p.tracker.Debug("downstream server closed connection")
				return
			}
		}
	}
}
