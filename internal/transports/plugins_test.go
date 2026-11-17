package transports

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gobwas/ws"
)

type callbackTransport struct{ deliver Deliver }

func (c callbackTransport) Write(key []byte, op ws.OpCode, p []byte) error {
	return c.deliver(key, op, p)
}

func TestHTTPPluginWireContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, []byte{byte(ws.OpBinary), 0, 255}) || r.Header.Get("X-Recipient") != "user" {
			t.Errorf("body %v, header %v: %v", body, r.Header, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	options := Options{Resolver: transportForServer(t, server).resolver, UserHeader: "X-Recipient"}
	for _, name := range []string{"", "http"} {
		backend, err := Open(name, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := backend.Write([]byte("user"), ws.OpBinary, []byte{0, 255}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompiledTransportAndFactoryErrors(t *testing.T) {
	payload := []byte("message")
	called := false
	options := Options{Deliver: func(key []byte, op ws.OpCode, p []byte) error {
		called = true
		if string(key) != "user" || op != ws.OpText || &p[0] != &payload[0] {
			t.Error("delivery contract changed")
		}
		return ErrRecipientNotConnected
	}}
	failed := errors.New("connect failed")
	plugins := map[string]Factory{
		"callback": func(o Options) (Transport, error) { return callbackTransport{o.Deliver}, nil },
		"broken":   func(Options) (Transport, error) { return nil, failed },
		"nil":      func(Options) (Transport, error) { return nil, nil },
	}
	backend, err := Open("callback", options, plugins)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Write([]byte("user"), ws.OpText, payload); !errors.Is(err, ErrRecipientNotConnected) || !called {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "nil", "http"} {
		if _, err := Open(name, Options{}, plugins); err == nil {
			t.Fatalf("accepted invalid %s", name)
		}
	}
	if _, err := Open("broken", options, plugins); !errors.Is(err, failed) {
		t.Fatal(err)
	}
}
