package amqplink

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/transports/link"
)

func brokerConfig(t *testing.T) Config {
	t.Helper()
	url := os.Getenv("LAVINMQ_AMQP_URL")
	if url == "" {
		t.Skip("set LAVINMQ_AMQP_URL for real-broker tests")
	}
	return Config{URL: url, Address: "127.0.0.1:3000", Prefix: fmt.Sprintf("link-test.%d", time.Now().UnixNano()), Timeout: time.Second, Heartbeat: 100 * time.Millisecond, Lease: 500 * time.Millisecond}
}
func openEndpoint(t *testing.T, c Config) *Endpoint {
	t.Helper()
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestBrokerBidirectionalSession(t *testing.T) {
	c := brokerConfig(t)
	closed := make(chan struct{}, 1)
	c.Accept = func(user string, s link.Session) error {
		go func() {
			defer func() { s.Close(); closed <- struct{}{} }()
			s.Write(ws.OpText, []byte("hello "+user))
			body, op, err := s.Read()
			if err == nil {
				s.Write(op, body)
			}
			s.Read()
		}()
		return nil
	}
	openEndpoint(t, c)
	c.Address = "127.0.0.1:8080"
	c.Accept = nil
	lb := openEndpoint(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := lb.Dial(ctx, "127.0.0.1:3000", "recipient")
	if err != nil {
		t.Fatal(err)
	}
	// Prevent a failing implementation from leaving a blocked read until go test times out.
	timer := time.AfterFunc(3*time.Second, func() { lb.Close() })
	defer timer.Stop()
	body, op, err := session.Read()
	if err != nil || op != ws.OpText || string(body) != "hello recipient" {
		t.Fatalf("greeting %q/%v: %v", body, op, err)
	}
	if err := session.Write(ws.OpBinary, []byte{0, 255}); err != nil {
		t.Fatal(err)
	}
	body, op, err = session.Read()
	if err != nil || op != ws.OpBinary || string(body) != string([]byte{0, 255}) {
		t.Fatalf("reply %v/%v: %v", body, op, err)
	}
	session.Close()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("remote application was not closed")
	}
	if _, err := lb.Dial(ctx, "127.0.0.1:3002", "missing"); err == nil {
		t.Fatal("missing sidecar accepted")
	}
	if len(lb.snapshot()) != 0 {
		t.Fatal("failed or closed session retained")
	}
}
