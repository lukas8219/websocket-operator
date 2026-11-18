package main

import (
	"bytes"
	"context"
	"fmt"
	"lukas8219/websocket-operator/internal/transports/builtin"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
)

type pluginResolver struct {
	resolver.Resolver
	peer peer_discovery.Peer
}

func (r pluginResolver) Lookup([]byte) (peer_discovery.Peer, error) {
	if r.peer.Port() != 0 {
		return r.peer, nil
	}
	return peer_discovery.NewPeer("127.0.0.1", 3000), nil
}

func TestLavinMQPluginToApplication(t *testing.T) {
	url := os.Getenv("LAVINMQ_AMQP_URL")
	if url == "" {
		t.Skip("set LAVINMQ_AMQP_URL for real-broker tests")
	}
	t.Setenv("WS_OPERATOR_LAVINMQ_URL", url)
	for _, namespace := range []string{"amq", strings.Repeat("n", 63)} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv("WS_OPERATOR_NAMESPACE", namespace)
			t.Setenv("WS_OPERATOR_SERVICE", strings.Repeat("s", 46)+fmt.Sprintf("%016x", time.Now().UnixNano()))
			t.Setenv("WS_OPERATOR_POD_IP", "127.0.0.1")
			t.Setenv("WS_OPERATOR_TRANSPORT_PLUGIN", "lavinmq")
			config, err := builtin.FromEnv("3000")
			if err != nil {
				t.Fatal(err)
			}
			registry := newConnectionRegistry()
			local, app := net.Pipe()
			defer local.Close()
			defer app.Close()
			app.SetDeadline(time.Now().Add(3 * time.Second))
			registry.register(&ConnectionTracker{user: "recipient", upstreamConn: local})
			backend, err := builtin.Open(config, builtin.Options{Resolver: pluginResolver{}, Deliver: localDelivery(registry)})
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			if err := backend.Write([]byte("recipient"), ws.OpText, []byte("through broker")); err != nil {
				t.Fatal(err)
			}
			body, op, err := wsutil.ReadClientData(app)
			if err != nil || op != ws.OpText || string(body) != "through broker" {
				t.Fatalf("%q/%v: %v", body, op, err)
			}
		})
	}
}

func TestLavinMQSelectedBySidecarProcess(t *testing.T) {
	url := os.Getenv("LAVINMQ_AMQP_URL")
	if url == "" {
		t.Skip("set LAVINMQ_AMQP_URL for real-broker tests")
	}
	service := fmt.Sprintf("plugin-%d", time.Now().UnixNano())
	t.Setenv("WS_OPERATOR_TRANSPORT_PLUGIN", "lavinmq")
	t.Setenv("WS_OPERATOR_LAVINMQ_URL", url)
	t.Setenv("WS_OPERATOR_POD_IP", "127.0.0.1")
	t.Setenv("WS_OPERATOR_NAMESPACE", "test")
	t.Setenv("WS_OPERATOR_SERVICE", service)
	fixture := startBehaviorSidecar(t)
	host, port, _ := net.SplitHostPort(fixture.url)
	number, _ := strconv.Atoi(port)
	transportConfig, err := builtin.FromEnv("1")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := builtin.Open(transportConfig, builtin.Options{Resolver: pluginResolver{peer: peer_discovery.NewPeer(host, uint16(number))}})
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	timer := time.AfterFunc(5*time.Second, func() { sender.Close() })
	defer timer.Stop()
	client, err := sender.Dial(context.Background(), fixture.url, "recipient")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Write(ws.OpText, []byte(`{"recipientId":"recipient"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Read(); err != nil {
		t.Fatal(err)
	}
	payload := []byte{0, 1, 255}
	if err := sender.Write([]byte("recipient"), ws.OpBinary, payload); err != nil {
		t.Fatal(err)
	}
	body, op, err := client.Read()
	if err != nil || op != ws.OpBinary || !bytes.Equal(body, payload) {
		t.Fatalf("process delivery %v/%v: %v", body, op, err)
	}
	// The broker mode must not silently keep a direct WebSocket session path.
	direct := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{"ws-user-id": []string{"recipient"}})}
	if conn, _, _, err := direct.Dial(context.Background(), "ws://"+fixture.url); err == nil {
		conn.Close()
		t.Fatal("broker endpoint accepted a direct session")
	}
}
