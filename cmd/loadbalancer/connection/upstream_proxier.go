package connection

import (
	"context"
	"errors"
	"log/slog"
	"net"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (p *WSProxier) ProxyUpstreamToDownstream() {
	go _proxyUpstreamToDownstream(p.tracker.UpstreamContext(), p.tracker.User(), p.tracker.DownstreamConn(), p.tracker.UpstreamConn())
}

func _proxyUpstreamToDownstream(ctx context.Context, user string, downstreamConn net.Conn, upstreamConn net.Conn) {
	defer upstreamConn.Close()
	defer func() {
		if ctx.Err() == nil {
			downstreamConn.Close()
		}
	}()
	log := slog.With("user", user).
		With("upstreamHost", upstreamConn.RemoteAddr().String()).
		With("downstreamHost", downstreamConn.RemoteAddr().String())
	for {
		select {
		case <-ctx.Done():
			log.Debug("downstream to upstream context done")
			return
		default:
			msg, op, err := wsutil.ReadClientData(downstreamConn)
			if err != nil {
				log.Error("Failed to read from downstream", "error", err)
				var protocolError ws.ProtocolError
				if errors.As(err, &protocolError) {
					wsutil.WriteServerMessage(downstreamConn, ws.OpClose, ws.NewCloseFrameBody(ws.StatusProtocolError, ""))
				} else if errors.Is(err, wsutil.ErrInvalidUTF8) {
					wsutil.WriteServerMessage(downstreamConn, ws.OpClose, ws.NewCloseFrameBody(ws.StatusInvalidFramePayloadData, ""))
				}
				return
			}
			err = wsutil.WriteClientMessage(upstreamConn, op, msg)
			if err != nil {
				log.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				log.Debug("downstream server closed connection")
				return
			}
		}
	}

}
