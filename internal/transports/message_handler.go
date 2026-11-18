package transports

import (
	"errors"
	"io"
	"net/http"

	"github.com/gobwas/ws"
)

// MessageHandler receives the direct backend's cross-sidecar deliveries.
func MessageHandler(header string, deliver Deliver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/message" {
			next.ServeHTTP(w, r)
			return
		}
		if deliver == nil {
			http.Error(w, "local delivery unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			http.Error(w, "message body must include an opcode", http.StatusBadRequest)
			return
		}
		err = deliver([]byte(r.Header.Get(header)), ws.OpCode(body[0]), body[1:])
		if errors.Is(err, ErrRecipientNotConnected) {
			http.Error(w, "recipient not connected", http.StatusNotFound)
		} else if err != nil {
			http.Error(w, "application delivery failed", http.StatusBadGateway)
		}
	})
}
