package main

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/routing"
)

type transportFunc func([]byte, ws.OpCode, []byte) error

func (f transportFunc) Write(key []byte, op ws.OpCode, body []byte) error {
	return f(key, op, body)
}

func boundaryPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	for _, conn := range []net.Conn{a, b} {
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		t.Cleanup(func() { conn.Close() })
	}
	return a, b
}

func TestTransportOnlyHandlesMissingLocalRecipient(t *testing.T) {
	for _, placement := range []string{"local", "remote"} {
		for _, op := range []ws.OpCode{ws.OpText, ws.OpBinary} {
			t.Run(fmt.Sprintf("%s/op=%d", placement, op), func(t *testing.T) {
				client, incoming := boundaryPipe(t)
				outgoing, application := boundaryPipe(t)
				local, remote := newConnectionRegistry(), newConnectionRegistry()
				target := local
				if placement == "remote" {
					target = remote
				}
				target.register(&ConnectionTracker{user: "to", upstreamConn: outgoing})
				calls := 0
				body := []byte("to:hello")
				if op == ws.OpBinary {
					body = []byte{'t', 'o', 0, 255}
				}
				plugin := transportFunc(func(key []byte, gotOp ws.OpCode, payload []byte) error {
					calls++
					if string(key) != "to" || gotOp != op || !bytes.Equal(payload, body) {
						t.Errorf("changed routed message: %q/%v/%v", key, gotOp, payload)
					}
					return localDelivery(remote)(key, gotOp, payload)
				})
				extractor := routing.ExtractorFunc(func(_ ws.OpCode, p []byte) ([]byte, error) { return p[:2], nil })
				tracker := &ConnectionTracker{downstreamConn: incoming, Transport: plugin}
				done := make(chan struct{})
				go handleIncomingMessagesToProxy(local, func() { close(done) }, tracker, extractor)
				if err := wsutil.WriteClientMessage(client, op, body); err != nil {
					t.Fatal(err)
				}
				got, gotOp, err := wsutil.ReadClientData(application)
				if err != nil || gotOp != op || !bytes.Equal(got, body) {
					t.Fatalf("application received %v/%v: %v", got, gotOp, err)
				}
				client.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("incoming reader did not stop")
				}
				want := 0
				if placement == "remote" {
					want = 1
				}
				if calls != want {
					t.Fatalf("transport called %d times, want %d", calls, want)
				}
			})
		}
	}
}

func TestApplicationRepliesUseExistingWebSocket(t *testing.T) {
	application, upstream := boundaryPipe(t)
	downstream, loadbalancer := boundaryPipe(t)
	tracker := &ConnectionTracker{upstreamConn: upstream, downstreamConn: downstream,
		Transport: transportFunc(func([]byte, ws.OpCode, []byte) error {
			t.Error("ordinary application reply used the routing transport")
			return nil
		}),
	}
	done := make(chan struct{})
	go proxySidecarServerToClient(func() { close(done) }, tracker)
	if err := wsutil.WriteServerMessage(application, ws.OpBinary, []byte{0, 255}); err != nil {
		t.Fatal(err)
	}
	body, op, err := wsutil.ReadServerData(loadbalancer)
	if err != nil || op != ws.OpBinary || !bytes.Equal(body, []byte{0, 255}) {
		t.Fatalf("reply %v/%v: %v", body, op, err)
	}
	application.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("application reader did not stop")
	}
}
