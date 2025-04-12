package proxy

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"lukas8219/websocket-operator/internal/route"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

var (
	router route.RouterImpl
)

func InitializeProxy(mode string) {
	router = route.NewRouter(route.RouterConfig{Mode: route.RouterConfigMode(mode)})
	err := router.InitializeHosts()
	if err != nil {
		slog.Error("failed to initialize hosts", "error", err)
		//TODO should we panic here?
	}
}

func SendProxiedMessage(recipientId string, message []byte, opCode ws.OpCode) error {
	host := router.Route(recipientId)
	slog := slog.With("recipientId", recipientId).With("opCode", opCode).With("host", host).With("component", "proxy")
	if host == "" {
		slog.Error("no host found")
		return errors.New("no host found")
	}
	slog.Debug("Routing message")
	//TODO hardcoded 5 seconds to debug DNS resolve issues
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	messageWithOpCode := append([]byte{byte(opCode)}, message...)
	url := "http://" + host + "/message"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(messageWithOpCode))
	if err != nil {
		slog.Error("failed to create request", "error", err)
		return errors.Join(errors.New("failed to create request"), err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ws-user-id", recipientId)

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
