package amqplink

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/transports/link"
)

func waitSessions(t *testing.T, e *Endpoint, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(e.snapshot()) == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sessions=%d, want %d", len(e.snapshot()), count)
}
func TestTimedOutOpenCannotResurrect(t *testing.T) {
	c := brokerConfig(t)
	entered := make(chan link.Session, 1)
	release := make(chan struct{})
	var once sync.Once
	releaseSetup := func() { once.Do(func() { close(release) }) }
	defer releaseSetup()
	returned := make(chan struct{})
	c.Accept = func(_ string, s link.Session) error { entered <- s; <-release; close(returned); return nil }
	server := openEndpoint(t, c)
	c.Address = "127.0.0.1:8080"
	c.Accept = nil
	owner := openEndpoint(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if s, err := owner.Dial(ctx, server.config.Address, "slow"); !errors.Is(err, context.DeadlineExceeded) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("open: %v", err)
	}
	var pending link.Session
	select {
	case pending = <-entered:
	case <-time.After(time.Second):
		t.Fatal("application setup did not start")
	}
	waitSessions(t, owner, 0)
	waitSessions(t, server, 0)
	if _, _, err := pending.Read(); err == nil {
		t.Fatal("expired application session is readable")
	}
	// Returning from delayed setup must not make the closed session writable.
	releaseSetup()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("setup did not return")
	}
	if err := pending.Write(ws.OpText, []byte("late")); err == nil {
		t.Fatal("late setup revived session")
	}
}
func TestSilentSessionExpiresWhileSamePeerStaysAlive(t *testing.T) {
	c := brokerConfig(t)
	c.Accept = func(_ string, s link.Session) error {
		go func() {
			defer s.Close()
			for {
				body, op, err := s.Read()
				if err != nil {
					return
				}
				if s.Write(op, body) != nil {
					return
				}
			}
		}()
		return nil
	}
	server := openEndpoint(t, c)
	c.Address = "127.0.0.1:8080"
	c.Accept = nil
	owner := openEndpoint(t, c)
	lost, err := owner.Dial(context.Background(), server.config.Address, "lost")
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := owner.Dial(context.Background(), server.config.Address, "healthy")
	if err != nil {
		t.Fatal(err)
	}
	defer healthy.Close()
	// Simulate a lost Close packet, while keeping the owner's broker connection
	// and another session alive. Per-peer heartbeats would leak the old session.
	lost.(*session).stop(io.EOF)
	waitSessions(t, server, 1)
	timer := time.AfterFunc(2*time.Second, func() { owner.Close() })
	defer timer.Stop()
	if err := healthy.Write(ws.OpBinary, []byte{0, 255}); err != nil {
		t.Fatal(err)
	}
	body, op, err := healthy.Read()
	if err != nil || op != ws.OpBinary || string(body) != string([]byte{0, 255}) {
		t.Fatalf("healthy reply %v/%v: %v", body, op, err)
	}
	owner.wire.Close()
	select {
	case <-owner.Done():
	case <-time.After(time.Second):
		t.Fatal("broker loss did not close endpoint")
	}
	if _, _, err := healthy.Read(); err == nil {
		t.Fatal("broker loss left a session open")
	}
	waitSessions(t, owner, 0)
	waitSessions(t, server, 0)
}
