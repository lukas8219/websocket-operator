package connection

import (
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (p *WSProxier) ProxyDownstreamToUpstream() (net.Conn, error) {
	host := p.tracker.UpstreamHost()
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"ws-user-id": []string{p.tracker.User()},
		},
	}
	p.tracker.Debug("Dialing upstream")
	proxiedConn, _, _, err := dialer.Dial(p.tracker.UpstreamContext(), "ws://"+host)
	if err != nil {
		p.tracker.Error("Failed to dial upstream", "error", err)
		return nil, err
	}
	p.tracker.Debug("Connected to upstream")
	p.tracker.SetUpstreamConn(proxiedConn)

	waitSignal := func() {
		p.tracker.Debug("Closing upstream connection")
		select {
		case p.tracker.UpstreamCancelChan() <- 1:
			p.tracker.Debug("Successfully signaled cancellation")
		default:
			p.tracker.Debug("No one waiting for cancellation signal, skipping")
		}
	}

	//TODO missing defer
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-p.tracker.UpstreamContext().Done():
				p.tracker.Debug("Upstream context done")
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
				err = wsutil.WriteServerMessage(p.tracker.DownstreamConn(), op, msg)
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
