package route

import (
	"bytes"
	"errors"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"

	"github.com/gobwas/ws"
)

func getRandomSRVHost(service string) (string, error) {
	log.Println("Getting random SRV host for service:", service)
	_, addrs, err := net.LookupSRV("", "", service)
	log.Println("Addrs:", addrs)
	if err != nil {
		return "", err
	}

	if len(addrs) == 0 {
		return "", nil
	}

	// Pick a random address from the list
	selected := addrs[rand.Intn(len(addrs))]
	return net.JoinHostPort(selected.Target, string(selected.Port)), nil
}

func Route(recipientId string, message []byte, opCode ws.OpCode) error {
	srvRecord := os.Getenv("WS_OPERATOR_SRV_DNS_RECORD")
	if srvRecord == "" {
		srvRecord = "ws-operator"
	}
	host, err := getRandomSRVHost(srvRecord)
	log.Println("Host:", host)
	if err != nil {
		return err
	}

	if host == "" {
		return errors.New("no host found")
	}

	//KeepAlive
	//Batch POST in case of high volume
	req, err := http.NewRequest("POST", "http://"+host+"/message", bytes.NewReader(message))
	if err != nil {
		return errors.New("failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WS-Operation", "1")
	req.Header.Set("ws-user-id", recipientId)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	log.Println("Response:", resp)
	defer resp.Body.Close()
	return nil
}
