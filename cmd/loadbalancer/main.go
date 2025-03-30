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
	router = *route.NewRouter()
)

func init() {
	err := router.InitializeHosts()
	if err != nil {
		log.Println(errors.Join(errors.New("failed to initialize hosts"), err))
	}
}

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	flag.Parse()

	log.Printf("Starting load balancer server on port %s", *port)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("ws-user-id")
		if user == "" {
			log.Println("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Println("New connection from", user)
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println(err)
		}
		host := router.Route(user)
		log.Println("Dialing upstream", host)
		dialer := ws.Dialer{
			Header: ws.HandshakeHeaderHTTP{
				"ws-user-id": []string{user},
			},
		}
		proxiedConn, _, _, err := dialer.Dial(context.Background(), "ws://"+host)
		if err != nil {
			log.Println(errors.Join(errors.New("failed to dial upstream"), err))
			clientConn.Close()
			return
		}
		closeConnections := func() {
			clientConn.Close()
			proxiedConn.Close()
		}
		handleIncomingMessagesToProxy := func(clientConnection net.Conn, targetConnection net.Conn) {
			defer closeConnections()
			for {
				msg, op, err := wsutil.ReadClientData(clientConnection)
				if err != nil {
					log.Println(errors.Join(err, errors.New("failed to read from client")))
					return
				}
				err = wsutil.WriteClientMessage(targetConnection, op, msg)
				if err != nil {
					log.Println(errors.Join(err, errors.New("failed to write to client")))
					return
				}
				if op == ws.OpClose {
					log.Println("Client closed connection")
					return
				}
				log.Println("Proxied client message")
			}
		}
		proxySidecarServerToClient := func(serverConnection net.Conn, targetConnection net.Conn) {
			defer closeConnections()
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
				log.Println("Proxied server message")
			}
		}
		go proxySidecarServerToClient(proxiedConn, clientConn)
		go handleIncomingMessagesToProxy(clientConn, proxiedConn)
	}))
}
