package main

import (
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/routing"
	"lukas8219/websocket-operator/internal/transports/link"
)

func TestBinaryRoutingPluginPreservesPayload(t *testing.T) {
	client, input := net.Pipe()
	output, app := net.Pipe()
	defer client.Close()
	defer input.Close()
	defer output.Close()
	defer app.Close()
	for _, c := range []net.Conn{client, input, output, app} {
		c.SetDeadline(time.Now().Add(3 * time.Second))
	}
	registry := newConnectionRegistry()
	registry.register(&ConnectionTracker{user: "to", upstreamConn: output})
	tracker := &ConnectionTracker{downstreamConn: link.WebSocket(input, ws.StateServerSide)}
	extractor := routing.ExtractorFunc(func(op ws.OpCode, p []byte) ([]byte, error) {
		if op != ws.OpBinary || len(p) < 2 {
			t.Error("unexpected input")
		}
		return p[:2], nil
	})
	done := make(chan struct{})
	go handleIncomingMessagesToProxy(registry, func() { close(done) }, tracker, extractor)
	sent := make(chan error, 1)
	go func() { sent <- wsutil.WriteClientMessage(client, ws.OpBinary, []byte("to\x00payload")) }()
	payload, op, err := wsutil.ReadClientData(app)
	if err != nil || op != ws.OpBinary || string(payload) != "to\x00payload" {
		t.Fatalf("%q/%v: %v", payload, op, err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not stop")
	}
}
