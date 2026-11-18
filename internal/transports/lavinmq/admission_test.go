package lavinmq

import (
	"context"
	"errors"
	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"testing"
	"time"
)

func TestAdmissionTimeoutKeepsTransportAlive(t *testing.T) {
	p := &Transport{config: Config{Timeout: time.Millisecond, Lookup: func([]byte) (peer_discovery.Peer, error) { return peer_discovery.Peer{}, nil }}, gate: make(chan struct{}, 1), done: make(chan struct{})}
	p.gate <- struct{}{}
	if err := p.Write([]byte("user"), ws.OpText, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
		t.Fatal("admission timeout killed transport without publishing")
	default:
	}
}

func TestCancelledCallerDoesNotStartPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &Transport{config: Config{Timeout: time.Second, Lookup: func([]byte) (peer_discovery.Peer, error) { return peer_discovery.Peer{}, nil }}, gate: make(chan struct{}, 1), done: make(chan struct{})}
	if err := p.WriteContext(ctx, []byte("user"), ws.OpText, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
		t.Fatal("cancelled admission closed transport")
	default:
	}
}
