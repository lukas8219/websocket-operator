package proxy

import (
	"bytes"
	"context"
	"errors"
	"log"
	"lukas8219/websocket-operator/internal/route"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

var (
	router *route.Router = route.NewRouter()
)

// Stop doing this kind of side-effect in `init`
func init() {
	err := router.InitializeHosts()
	if err != nil {
		log.Println(errors.Join(errors.New("failed to initialize hosts"), err))
	}
}

func SendProxiedMessage(recipientId string, message []byte, opCode ws.OpCode) error {
	host := router.Route(recipientId)

	log.Println("Host:", host)
	if host == "" {
		return errors.New("no host found")
	}

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
		log.Println("Error sending request:", err)
		return err
	}
	log.Println("Response:", resp)
	defer resp.Body.Close()
	return nil
}
