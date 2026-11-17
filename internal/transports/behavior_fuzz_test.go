package transports

import (
	"bytes"
	"encoding/hex"
	"github.com/gobwas/ws"
	"io"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

type transportResolver struct {
	resolver.Resolver
	peer peer_discovery.Peer
}

func (r transportResolver) Lookup([]byte) (peer_discovery.Peer, error) { return r.peer, nil }
func transportForServer(t testing.TB, s *httptest.Server) HttpTransport {
	t.Helper()
	host, portText, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return NewHTTPTransport(transportResolver{peer: peer_discovery.NewPeer(host, uint16(port))}, "ws-user-id")
}

func FuzzHTTPTransportEnvelope(f *testing.F) {
	f.Add([]byte("recipient"), []byte("hello"), uint8(1))
	f.Add([]byte{}, []byte{}, uint8(2))
	f.Fuzz(func(t *testing.T, id, payload []byte, opcode uint8) {
		if len(id) > 64 || len(payload) > 2048 {
			t.Skip()
		}
		recipient := hex.EncodeToString(id)
		type request struct {
			body         []byte
			id           string
			method, path string
			err          error
		}
		got := make(chan request, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, err := io.ReadAll(r.Body)
			got <- request{b, r.Header.Get("ws-user-id"), r.Method, r.URL.Path, err}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		transport := transportForServer(t, server)
		if err := transport.Write([]byte(recipient), ws.OpCode(opcode), payload); err != nil {
			t.Fatal(err)
		}
		observed := <-got
		want := append([]byte{opcode}, payload...)
		if observed.err != nil || !bytes.Equal(observed.body, want) || observed.id != recipient || observed.method != "POST" || observed.path != "/message" {
			t.Fatalf("corrupted envelope: %+v", observed)
		}
	})
}
