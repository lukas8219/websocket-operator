package link

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
)

func TestDirectBidirectionalSession(t *testing.T) {
	server, err := NewDirect("X-App-User", func(user string, session Session) error {
		if user != "recipient" {
			t.Errorf("wrong user %q", user)
		}
		go func() {
			defer session.Close()
			if err := session.Write(ws.OpText, []byte("welcome")); err != nil {
				return
			}
			body, op, err := session.Read()
			if err == nil {
				session.Write(op, body)
			}
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, _ := NewDirect("X-App-User", nil)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := client.Dial(ctx, strings.TrimPrefix(httpServer.URL, "http://"), "recipient")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	body, op, err := session.Read()
	if err != nil || op != ws.OpText || string(body) != "welcome" {
		t.Fatalf("greeting %q/%v: %v", body, op, err)
	}
	if err := session.Write(ws.OpBinary, []byte{0, 255}); err != nil {
		t.Fatal(err)
	}
	body, op, err = session.Read()
	if err != nil || op != ws.OpBinary || string(body) != string([]byte{0, 255}) {
		t.Fatalf("reply %v/%v: %v", body, op, err)
	}
}

func TestDirectEndpointCloseReleasesSessions(t *testing.T) {
	accepted := make(chan Session, 1)
	server, _ := NewDirect("ws-user-id", func(_ string, s Session) error { accepted <- s; return nil })
	defer server.Close()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, _ := NewDirect("ws-user-id", nil)
	defer client.Close()
	session, err := client.Dial(context.Background(), strings.TrimPrefix(httpServer.URL, "http://"), "user")
	if err != nil {
		t.Fatal(err)
	}
	<-accepted
	read := make(chan error, 1)
	go func() { _, _, err := session.Read(); read <- err }()
	server.Close()
	server.Close()
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("closed endpoint left readable session")
		}
	case <-time.After(time.Second):
		t.Fatal("session survived endpoint shutdown")
	}
	client.Close()
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.sessions) != 0 {
		t.Fatal("closed sessions retained")
	}
}
