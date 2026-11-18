package link

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type webSocket struct {
	conn      net.Conn
	state     ws.State
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// WebSocket adapts either side of a WebSocket to the common message contract.
// The caller must preserve any bytes buffered during the HTTP handshake.
func WebSocket(conn net.Conn, state ws.State) Session {
	return &webSocket{conn: conn, state: state}
}

func (s *webSocket) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.conn.Close() })
	return s.closeErr
}

func (s *webSocket) write(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err == nil {
		err = fn()
	}
	if err != nil {
		s.Close()
	}
	return err
}

func (s *webSocket) Write(op ws.OpCode, body []byte) error {
	if op != ws.OpText && op != ws.OpBinary {
		return errors.New("session requires a data message")
	}
	return s.write(func() error { return wsutil.WriteMessage(s.conn, s.state, op, body) })
}

type controlWriter struct{ socket *webSocket }

func (w controlWriter) Write(p []byte) (n int, err error) {
	err = w.socket.write(func() error { n, err = w.socket.conn.Write(p); return err })
	return n, err
}

func (s *webSocket) Read() ([]byte, ws.OpCode, error) {
	rw := struct {
		io.Reader
		io.Writer
	}{s.conn, controlWriter{s}}
	body, op, err := wsutil.ReadData(rw, s.state)
	if err != nil {
		var protocol ws.ProtocolError
		var status ws.StatusCode
		if errors.As(err, &protocol) {
			status = ws.StatusProtocolError
		}
		if errors.Is(err, wsutil.ErrInvalidUTF8) {
			status = ws.StatusInvalidFramePayloadData
		}
		if status != 0 {
			s.write(func() error {
				return wsutil.WriteMessage(s.conn, s.state, ws.OpClose, ws.NewCloseFrameBody(status, ""))
			})
		}
	}
	return body, op, err
}
