package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"lukas8219/websocket-operator/internal/transports/builtin"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/transports"
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
			factory := lavinMQFactory("127.0.0.1:3000", config.Discovery{Namespace: namespace, Service: strings.Repeat("s", 46) + fmt.Sprintf("%016x", time.Now().UnixNano())})
			registry := newConnectionRegistry()
			local, app := net.Pipe()
			defer local.Close()
			defer app.Close()
			app.SetDeadline(time.Now().Add(3 * time.Second))
			registry.register(&ConnectionTracker{user: "recipient", upstreamConn: local})
			backend, err := transports.Open("lavinmq", transports.Options{Resolver: pluginResolver{}, Deliver: localDelivery(registry)}, map[string]transports.Factory{"lavinmq": factory})
			if err != nil {
				t.Fatal(err)
			}
			defer backend.(interface{ Close() error }).Close()
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

type stoppingTransport struct {
	done   chan struct{}
	closed atomic.Int32
}

func (*stoppingTransport) Write([]byte, ws.OpCode, []byte) error { return nil }
func (p *stoppingTransport) Done() <-chan struct{}               { return p.done }
func (p *stoppingTransport) Close() error                        { p.closed.Add(1); return nil }

func TestPluginLifecycle(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "broker loss"}[failure], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend := &stoppingTransport{done: make(chan struct{})}
			server := &http.Server{Addr: "127.0.0.1:0"}
			ended := make(chan error, 1)
			go func() { ended <- serveTransport(ctx, server, backend) }()
			if failure {
				close(backend.done)
			} else {
				cancel()
			}
			select {
			case err := <-ended:
				if !errors.Is(err, http.ErrServerClosed) || backend.closed.Load() != 1 {
					t.Fatalf("shutdown: %v, closes=%d", err, backend.closed.Load())
				}
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
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
