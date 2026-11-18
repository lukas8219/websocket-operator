package main

import (
	"io"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// Hold one lock across both the frame header and payload, including automatic
// pong/close responses emitted by ReadServerData. net.Conn's concurrent safety
// alone does not make a multi-Write WebSocket frame atomic.
func (c *ConnectionTracker) writeApplication(write func() error) error {
	c.applicationMu.Lock()
	defer c.applicationMu.Unlock()
	err := c.upstreamConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err == nil {
		err = write()
	}
	if err != nil {
		c.upstreamConn.Close()
	}
	return err
}

func (c *ConnectionTracker) deliverApplication(op ws.OpCode, payload []byte) error {
	return c.writeApplication(func() error { return wsutil.WriteClientMessage(c.upstreamConn, op, payload) })
}

type applicationControlWriter struct{ connection *ConnectionTracker }

func (w applicationControlWriter) Write(payload []byte) (n int, err error) {
	err = w.connection.writeApplication(func() error {
		n, err = w.connection.upstreamConn.Write(payload)
		return err
	})
	return n, err
}

func (c *ConnectionTracker) applicationReader() io.ReadWriter {
	return struct {
		io.Reader
		io.Writer
	}{c.upstreamConn, applicationControlWriter{c}}
}
