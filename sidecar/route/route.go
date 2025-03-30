package route

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gobwas/ws"
)

var (
	router *Router = NewRouter()
)

func createResolver() *net.Resolver {

	// Create a custom resolver that first tries localhost:53 (for testing)
	// and falls back to the system resolver if that fails
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// First try localhost:53
			d := net.Dialer{}
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			log.Println("Looking for " + address + " on localhost:53")
			conn, err := d.DialContext(ctx, "udp", "0.0.0.0:53")
			if err != nil {
				log.Println("Failed to connect to localhost:53, falling back to system resolver:", err)
				return d.DialContext(ctx, network, address)
			}
			return conn, nil
		},
	}

	return r
}

func getRandomSRVHost(recipientId string, service string) (string, error) {
	resolver := createResolver()
	log.Println("Getting random SRV host for service:", service)
	_, addrs, err := resolver.LookupSRV(context.Background(), "", "", service)
	log.Println("Addrs:", addrs)
	if err != nil {
		return "", err
	}

	if len(addrs) == 0 {
		return "", nil
	}

	// Create a slice of tuples [addr,port] from the SRV records
	addrPorts := make([]string, len(addrs))
	for i, srv := range addrs {
		addr, err := resolver.LookupIP(context.Background(), "ip", srv.Target)
		if err != nil {
			return "", err
		}
		addrPorts[i] = net.JoinHostPort(addr[0].String(), strconv.Itoa(int(srv.Port)))
	}

	// Use the router to select a host based on recipient ID
	router.Add(addrPorts)

	addrAndPort := router.Route(recipientId)
	if err != nil {
		return "", err
	}
	return addrAndPort, nil
}

func Route(recipientId string, message []byte, opCode ws.OpCode) error {
	srvRecord := os.Getenv("WS_OPERATOR_SRV_DNS_RECORD")
	if srvRecord == "" {
		srvRecord = "ws-operator.local"
	}
	host, err := getRandomSRVHost(recipientId, srvRecord)
	if err != nil {
		log.Println("Error getting SRV records:", err)
		return err
	}

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
