package transports

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	rslv "lukas8219/websocket-operator/internal/resolver"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

type HttpTransport struct {
	resolver rslv.Resolver
}

func NewHTTPTransport(
	resolver rslv.Resolver,
) HttpTransport {
	return HttpTransport{resolver}
}

func (h *HttpTransport) Write(
	Recipient []byte,
	OpCode ws.OpCode,
	Data []byte,
) error {
	peer, error := h.resolver.Lookup(Recipient)
	if error != nil {
		return error
	}
	slog := slog.With("recipientId", Recipient).With("opCode", OpCode).With("peer", peer).With("component", "proxy")
	//TODO hardcoded 5 seconds to debug DNS resolve issues
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	messageWithOpCode := append([]byte{byte(OpCode)}, Data...)
	url := "http://" + peer.SocketAddres() + "/message"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(messageWithOpCode))
	if err != nil {
		slog.Error("failed to create request", "error", err)
		return errors.Join(errors.New("failed to create request"), err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ws-user-id", string(Recipient))

	slog.Debug("POST request", "url", url)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Error sending request", "error", err)
		return err
	}
	slog.Debug("Received response", "status", resp.Status)
	defer resp.Body.Close()
	return nil
}
