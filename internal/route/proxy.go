package route

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gobwas/ws"
)

var (
	router RouterImpl = NewRouter(RouterConfig{Mode: RouterConfigModeDns})
)

func Route(recipientId string, message []byte, opCode ws.OpCode) error {
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
