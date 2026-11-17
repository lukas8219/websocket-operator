package transports

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gobwas/ws"
)

func TestCustomRecipientHeader(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- r.Header.Clone(); w.WriteHeader(204) }))
	defer server.Close()
	base := transportForServer(t, server)
	transport := NewHTTPTransport(base.resolver, "X-App-User")
	if err := transport.Write([]byte("recipient"), ws.OpText, []byte("message")); err != nil {
		t.Fatal(err)
	}
	headers := <-received
	if headers.Get("X-App-User") != "recipient" || headers.Get("ws-user-id") != "" {
		t.Fatalf("headers: %v", headers)
	}
}
