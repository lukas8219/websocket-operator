package main

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/transports"
)

func TestPluginDeliveryToApplication(t *testing.T) {
	registry := newConnectionRegistry()
	deliver := localDelivery(registry)
	if err := deliver([]byte("missing"), ws.OpBinary, nil); !errors.Is(err, transports.ErrRecipientNotConnected) {
		t.Fatal(err)
	}
	sender, app := net.Pipe()
	defer sender.Close()
	defer app.Close()
	sender.SetDeadline(time.Now().Add(3 * time.Second))
	app.SetDeadline(time.Now().Add(3 * time.Second))
	registry.register(&ConnectionTracker{user: "recipient", upstreamConn: sender})
	sent := make(chan error, 1)
	go func() { sent <- deliver([]byte("recipient"), ws.OpBinary, []byte{0, 255}) }()
	payload, op, err := wsutil.ReadClientData(app)
	if err != nil || op != ws.OpBinary || string(payload) != string([]byte{0, 255}) {
		t.Fatalf("%v/%v: %v", payload, op, err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}
