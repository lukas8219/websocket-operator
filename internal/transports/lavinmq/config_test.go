package lavinmq

import (
	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"net"
	"strings"
	"testing"
	"time"
)

func TestInvalidConfigurationDoesNotLeakCredentials(t *testing.T) {
	for _, url := range []string{"", "http://guest:private-password@localhost", "amqp://guest:private-password@localhost"} {
		_, err := New(Config{URL: url})
		if err == nil || strings.Contains(err.Error(), "private-password") {
			t.Fatalf("configuration error: %v", err)
		}
	}
}

func TestQueueIdentity(t *testing.T) {
	a := queueName("namespace.service", "10.0.0.1:3000")
	if a == queueName("namespace.service", "10.0.0.2:3000") || a == queueName("other.service", "10.0.0.1:3000") || len(a) > 255 {
		t.Fatal("queue identity must isolate endpoints and services")
	}
}

func TestHandshakeTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			<-finished
		}
	}()
	started := time.Now()
	_, err = New(Config{URL: "amqp://guest:guest@" + listener.Addr().String(), Address: "127.0.0.1:3000", Prefix: "test", Timeout: 100 * time.Millisecond,
		Lookup:  func([]byte) (peer_discovery.Peer, error) { return peer_discovery.Peer{}, nil },
		Deliver: func([]byte, ws.OpCode, []byte) error { return nil },
	})
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("unbounded handshake: %v", err)
	}
}
