package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/transports/link"
)

type sessionRoutes struct{ resolver.Resolver }

func (sessionRoutes) Lookup([]byte) (peer_discovery.Peer, error) {
	return peer_discovery.NewPeer("10.0.0.1", 3000), nil
}

type echoSession struct {
	done     chan struct{}
	messages chan []byte
	once     sync.Once
}

func (s *echoSession) Write(op ws.OpCode, p []byte) error {
	select {
	case s.messages <- append([]byte{byte(op)}, p...):
		return nil
	case <-s.done:
		return io.EOF
	}
}
func (s *echoSession) Read() ([]byte, ws.OpCode, error) {
	select {
	case p := <-s.messages:
		return p[1:], ws.OpCode(p[0]), nil
	case <-s.done:
		return nil, 0, io.EOF
	}
}
func (s *echoSession) Close() error { s.once.Do(func() { close(s.done) }); return nil }

type selectedEndpoint struct {
	link.Endpoint
	session *echoSession
	opened  chan [2]string
}

func (e selectedEndpoint) Dial(_ context.Context, peer, user string) (link.Session, error) {
	e.opened <- [2]string{peer, user}
	return e.session, nil
}

func TestHandlerUsesSelectedTransportForBothDirections(t *testing.T) {
	routes := resolver.Resolver(sessionRoutes{})
	registry := newConnectionRegistry()
	session := &echoSession{done: make(chan struct{}), messages: make(chan []byte, 1)}
	endpoint := selectedEndpoint{session: session, opened: make(chan [2]string, 1)}
	server := httptest.NewServer(createHandler(&routes, registry, "X-App-User", endpoint))
	defer server.Close()
	defer session.Close()
	dialer := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{"X-App-User": []string{"recipient"}})}
	client, _, _, err := dialer.Dial(context.Background(), "ws://"+strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	select {
	case args := <-endpoint.opened:
		if args != [2]string{"10.0.0.1:3000", "recipient"} {
			t.Fatal(args)
		}
	case <-time.After(time.Second):
		t.Fatal("plugin not selected")
	}
	if err := wsutil.WriteClientMessage(client, ws.OpBinary, []byte{0, 255}); err != nil {
		t.Fatal(err)
	}
	body, op, err := wsutil.ReadServerData(client)
	if err != nil || op != ws.OpBinary || string(body) != string([]byte{0, 255}) {
		t.Fatalf("plugin round trip: %v/%v %v", body, op, err)
	}
	// A backend-side close must close the external client and remove membership.
	session.Close()
	if _, _, err := wsutil.ReadServerData(client); err == nil {
		t.Fatal("client survived backend close")
	}
	deadline := time.Now().Add(time.Second)
	for len(registry.snapshot()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(registry.snapshot()) != 0 {
		t.Fatal("closed connection retained")
	}
}
