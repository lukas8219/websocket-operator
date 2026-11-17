package connection

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type bufferedDialer struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (d bufferedDialer) Dial(context.Context, string) (net.Conn, *bufio.Reader, ws.Handshake, error) {
	return d.conn, d.reader, ws.Handshake{}, nil
}

func TestProxyPreservesFrameBytesReadDuringHandshake(t *testing.T) {
	var wire bytes.Buffer
	payload := []byte("first upstream message")
	if err := ws.WriteFrame(&wire, ws.NewTextFrame(payload)); err != nil {
		t.Fatal(err)
	}
	upstream, echo := net.Pipe()
	downstream, client := net.Pipe()
	for _, conn := range []net.Conn{upstream, echo, downstream, client} {
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
	}
	// A handshake read may consume the frame header and only part of its body.
	reader := bufio.NewReader(io.MultiReader(bytes.NewReader(wire.Bytes()[:5]), upstream))
	if _, err := reader.Peek(5); err != nil {
		t.Fatal(err)
	}
	proxy := NewWSProxier(NewTracker("user", "echo", "client", downstream), bufferedDialer{upstream, reader})
	conn, err := proxy.ProxyDownstreamToUpstream()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	written := make(chan error, 1)
	go func() { _, err := echo.Write(wire.Bytes()[5:]); written <- err }()
	got, op, err := wsutil.ReadServerData(client)
	if err != nil || op != ws.OpText || !bytes.Equal(got, payload) {
		t.Fatalf("first frame: opcode=%v payload=%q error=%v", op, got, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}
