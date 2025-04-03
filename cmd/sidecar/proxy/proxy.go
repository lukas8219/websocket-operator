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
	}
}

func SendProxiedMessage(recipientId string, message []byte, opCode ws.OpCode) error {
	host := router.Route(recipientId)
	if host == "" {
		return errors.New("no host found")
	}
	slog.Debug("Routing message", "host", host)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	messageWithOpCode := append([]byte{byte(opCode)}, message...)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+host+"/message", bytes.NewReader(messageWithOpCode))
	if err != nil {
		return errors.Join(errors.New("failed to create request"), err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ws-user-id", recipientId)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Error sending request", "error", err)
		return err
	}
	slog.Debug("Received response", "status", resp.Status)
	defer resp.Body.Close()
	return nil
}
