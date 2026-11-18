package connection

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/transports/link"
)

func TestTransportSessionRebalanceKeepsOneClientReader(t *testing.T) {
	closed := make(chan string, 2)
	sidecar := func(name string) string {
		endpoint, _ := link.NewDirect("user", func(_ string, s link.Session) error {
			go func() {
				defer func() { s.Close(); closed <- name }()
				for {
					body, op, err := s.Read()
					if err != nil {
						return
					}
					if s.Write(op, append([]byte(name), body...)) != nil {
						return
					}
				}
			}()
			return nil
		})
		server := httptest.NewServer(endpoint.Handler())
		t.Cleanup(func() { endpoint.Close(); server.Close() })
		return strings.TrimPrefix(server.URL, "http://")
	}
	first, second := sidecar("one:"), sidecar("two:")
	endpoint, _ := link.NewDirect("user", nil)
	defer endpoint.Close()
	frontend, client := net.Pipe()
	defer client.Close()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	connection := NewSessionConnection("recipient", first, "client", frontend, endpoint)
	defer connection.Close()
	connection.Handle()
	for i, expected := range []string{"one:payload", "two:payload"} {
		if i == 1 {
			connection.SwitchUpstreamHost(second)
			connection.Handle()
		}
		if err := wsutil.WriteClientMessage(client, ws.OpBinary, []byte("payload")); err != nil {
			t.Fatal(err)
		}
		body, op, err := wsutil.ReadServerData(client)
		if err != nil || op != ws.OpBinary || string(body) != expected {
			t.Fatalf("%q/%v: %v", body, op, err)
		}
	}
	connection.Close()
	for i := 0; i < 2; i++ {
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("old or current sidecar session leaked")
		}
	}
	select {
	case <-connection.Done():
	default:
		t.Fatal("client lifecycle did not end")
	}
}
