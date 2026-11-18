package amqplink

import (
	"context"
	"testing"
	"time"

	"github.com/gobwas/ws"
)

func TestSessionOwnsReceivedBuffersAndReleasesBudget(t *testing.T) {
	b := &budget{limit: 8}
	removed := false
	s := newSession("id", "peer", "user", b, func(context.Context, envelope) error { return nil }, func(*session) { removed = true })
	s.activate()
	body := []byte("original")
	if err := s.offer(envelope{Kind: data, Opcode: ws.OpBinary, Payload: body}); err != nil {
		t.Fatal(err)
	}
	clear(body)
	if err := s.offer(envelope{Payload: []byte("x")}); err != errOverflow {
		t.Fatal("global byte budget ignored")
	}
	got, op, err := s.Read()
	if err != nil || op != ws.OpBinary || string(got) != "original" {
		t.Fatalf("borrowed buffer retained: %q/%v %v", got, op, err)
	}
	if b.used != 0 {
		t.Fatal("read did not release queued bytes")
	}
	for i := 0; i < 16; i++ {
		if err := s.offer(envelope{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.offer(envelope{}); err != errOverflow {
		t.Fatal("message count not bounded")
	}
	s.Close()
	s.Close()
	if !removed || b.used != 0 {
		t.Fatal("close retained state")
	}
	if err := s.offer(envelope{Payload: []byte("late")}); err == nil {
		t.Fatal("closed session accepted data")
	}
}

func TestSessionCloseUnblocksPendingOpenAndRead(t *testing.T) {
	for _, active := range []bool{false, true} {
		s := newSession("id", "peer", "user", &budget{limit: 8}, func(context.Context, envelope) error { return nil }, func(*session) {})
		if active {
			s.activate()
		}
		result := make(chan error, 1)
		go func() { _, _, err := s.Read(); result <- err }()
		s.Close()
		select {
		case err := <-result:
			if err == nil {
				t.Fatal("closed read succeeded")
			}
		case <-time.After(time.Second):
			t.Fatal("read blocked after close")
		}
		if err := s.Write(ws.OpText, []byte("late")); err == nil {
			t.Fatal("closed session wrote data")
		}
	}
}
