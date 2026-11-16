package transports

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	peer, err := h.resolver.Lookup(Recipient)
	if err != nil {
		return err
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
	// A redirect is not confirmation that the selected peer accepted delivery.
	// Copy the client so this policy does not alter other HTTP users.
	client := *http.DefaultClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Error sending request", "error", err)
		return err
	}
	slog.Debug("Received response", "status", resp.Status)
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("peer %s rejected message: HTTP %d", peer.SocketAddres(), resp.StatusCode)
	}
	return nil
}
