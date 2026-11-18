package amqplink

import (
	"context"
	"testing"
	"time"

	"github.com/gobwas/ws"
)

func TestPeerCannotCloseOrInjectIntoAnotherSession(t *testing.T) {
	h := newHub(hubOptions{timeout: time.Second, interval: time.Hour, lease: time.Hour}, func(context.Context, string, envelope) error { return nil })
	defer h.Close()
	s, err := h.allocate("00112233445566778899aabbccddeeff", "127.0.0.1:8080", "user")
	if err != nil {
		t.Fatal(err)
	}
	s.activate()
	for _, event := range []kind{data, closeSession, ready} {
		h.receive(envelope{Kind: event, ID: s.id, Source: "127.0.0.2:8080", Opcode: ws.OpText, Payload: []byte("wrong")})
	}
	if h.find(s.id) != s || len(s.incoming) != 0 {
		t.Fatal("another peer altered session")
	}
	h.receive(envelope{Kind: closeSession, ID: s.id, Source: s.peer})
	if h.find(s.id) != nil {
		t.Fatal("owner close did not remove session")
	}
	h.receive(envelope{Kind: data, ID: s.id, Source: s.peer, Opcode: ws.OpText, Payload: []byte("late")})
	if h.find(s.id) != nil {
		t.Fatal("late data resurrected session")
	}
}
