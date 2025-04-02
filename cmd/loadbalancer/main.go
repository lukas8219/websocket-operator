package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var (
	router route.RouterImpl
)

func InitializeLoadBalancer(mode string) {
	router = route.NewRouter(route.RouterConfig{Mode: route.RouterConfigMode(mode)})
	router.InitializeHosts()
}

type ConnectionTracker struct {
	upstreamHost   string
	upstreamConn   net.Conn
	downstreamConn net.Conn
}

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	flag.Parse()
	InitializeLoadBalancer(*mode)
	log.Printf("Starting load balancer server on port %s", *port)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("ws-user-id")
		if user == "" {
			log.Println("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Println("New connection from", user)

		host := router.Route(user)
		if host == "" {
			log.Println("No host found for user", user)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println(err)
		}

		log.Println("Dialing upstream", host)
		connectionTracker := &ConnectionTracker{
			upstreamHost:   host,
			downstreamConn: clientConn,
		}

		proxiedCon, err := connectToUpstreamAndProxyMessages(connectionTracker, user)
		connectionTracker.upstreamConn = proxiedCon

		router.OnHostRebalance(func(hosts []string) error {
			log.Println("OnHostRebalance", hosts)
			for _, affectedHost := range hosts {
				log.Println("Checking host", affectedHost, "versus", connectionTracker.upstreamHost)
				if connectionTracker.upstreamHost == affectedHost {
					connectionTracker.upstreamConn.Close()
					newHost := router.Route(user)
					connectionTracker.upstreamHost = newHost
					log.Println("Rebalancing host", host, "to", newHost)
					_, err = connectToUpstreamAndProxyMessages(connectionTracker, user)
					if err != nil {
						log.Println(errors.Join(err, errors.New("failed to reconnect to upstream")))
						connectionTracker.Close()
						return err
					}
				}
			}
			return nil
		})
		go handleIncomingMessagesToProxy(connectionTracker)
	}))
}

func (c *ConnectionTracker) Close() {
	if c.upstreamConn != nil {
		c.upstreamConn.Close()
	}
	if c.downstreamConn != nil {
		c.downstreamConn.Close()
	}
}

func handleIncomingMessagesToProxy(connectionTracker *ConnectionTracker) {
	defer connectionTracker.Close()
	for {
		clientConnection := connectionTracker.downstreamConn
		upstreamConnection := connectionTracker.upstreamConn
		msg, op, err := wsutil.ReadClientData(clientConnection)
		if err != nil {
			log.Println(errors.Join(err, errors.New("failed to read from client")))
			return
		}
		//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
		err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
		if err != nil {
			log.Println(errors.Join(err, errors.New("failed to write to client")))
			return
		}
		if op == ws.OpClose {
			log.Println("Client closed connection")
			return
		}
	}
}

func connectToUpstreamAndProxyMessages(connectionTracker *ConnectionTracker, user string) (net.Conn, error) {
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"ws-user-id": []string{user},
		},
	}
	proxiedConn, _, _, err := dialer.Dial(context.Background(), "ws://"+connectionTracker.upstreamHost)
	if err != nil {
		log.Println(errors.Join(errors.New("failed to dial upstream"), err))
		if connectionTracker.upstreamConn != nil {
			connectionTracker.upstreamConn.Close()
		}
		return nil, err
	}
	connectionTracker.upstreamConn = proxiedConn
	//Missing DEFER
	proxySidecarServerToClient := func(serverConnection net.Conn, targetConnection net.Conn) {
		for {
			//Read as client - from the server.
			msg, op, err := wsutil.ReadServerData(serverConnection)
			if err != nil {
				log.Println(errors.Join(err, errors.New("failed to read from server")))
				return
			}
			//Write as client - to the proxied connection
			err = wsutil.WriteServerMessage(targetConnection, op, msg)
			if err != nil {
				log.Println(errors.Join(err, errors.New("failed to write to client")))
				return
			}
			if op == ws.OpClose {
				log.Println("Server closed connection")
				return
			}
		}
	}
	go proxySidecarServerToClient(proxiedConn, connectionTracker.downstreamConn)
	return proxiedConn, nil
}
