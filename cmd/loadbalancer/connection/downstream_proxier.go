package connection

import (
	"context"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (p *WSProxier) ProxyDownstreamToUpstream() (net.Conn, error) {
	host := p.tracker.UpstreamHost()

	p.tracker.Debug("Dialing upstream")
	upstreamContext := p.tracker.UpstreamContext()
	upstreamCancelChan := p.tracker.UpstreamCancelChan()
	downstreamConn := p.tracker.DownstreamConn()

	proxiedConn, _, _, err := p.dialer.Dial(context.Background(), "ws://"+host)
	if err != nil {
		p.tracker.Error("Failed to dial upstream", "error", err)
		return nil, err
	}
	p.tracker.Debug("Connected to upstream")

	waitSignal := func() {
		p.tracker.Debug("Closing upstream connection")
		select {
		case upstreamCancelChan <- 1:
			p.tracker.Debug("Successfully signaled cancellation")
		default:
			p.tracker.Debug("No one waiting for cancellation signal, skipping")
		}
		err := proxiedConn.Close()
		if err != nil {
			p.tracker.Error("Failed to close upstream connection", "error", err)
		}
	}

	//TODO missing defer
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-upstreamContext.Done():
				p.tracker.Debug("upstream to downstream context done")
				proxiedConn.Close()
				return
			default:
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(proxiedConn)
				if err != nil {
					p.tracker.Error("Failed to read from upstream", "error", err)
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(downstreamConn, op, msg)
				if err != nil {
					p.tracker.Error("Failed to write to downstream", "error", err)
					return
				}
				if op == ws.OpClose {
					p.tracker.Debug("upstream server closed connection")
					return
				}
			}
		}
	}
	go proxySidecarServerToClient()
	return proxiedConn, nil
}
