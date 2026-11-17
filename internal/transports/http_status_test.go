package transports

import (
	"github.com/gobwas/ws"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func FuzzHTTPTransportResponseStatus(f *testing.F) {
	f.Add(uint16(0))
	f.Fuzz(func(t *testing.T, raw uint16) {
		status := 200 + int(raw%400)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		defer server.Close()
		transport := transportForServer(t, server)
		err := transport.Write([]byte("recipient"), ws.OpText, []byte("hello"))
		if status >= 300 && err == nil {
			t.Fatalf("HTTP %d reported as successful message delivery", status)
		}
		if status < 300 && err != nil {
			t.Fatalf("HTTP %d failed: %v", status, err)
		}
	})
}

func TestBehaviorHTTPTransportRejectsFailedDelivery(t *testing.T) {
	for _, status := range []int{404, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			transport := transportForServer(t, server)
			if err := transport.Write([]byte("recipient"), ws.OpText, []byte("message")); err == nil {
				t.Fatalf("HTTP %d incorrectly returned nil delivery error", status)
			}
		})
	}
}

func TestHTTPTransportDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			forwarded := make(chan struct{}, 1)
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded <- struct{}{}; w.WriteHeader(204) }))
			defer destination.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(status)
			}))
			defer source.Close()
			transport := transportForServer(t, source)
			if err := transport.Write([]byte("recipient"), ws.OpText, []byte("message")); err == nil {
				t.Fatalf("redirect %d reported as delivery", status)
			}
			select {
			case <-forwarded:
				t.Fatal("message was forwarded to redirect destination")
			default:
			}
		})
	}
}
