package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func main() {
	log.Println("Starting server on port 3000")
	http.ListenAndServe(":3000", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("New connection")
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println(err)
		}
		proxiedConn, _, _, err := ws.Dial(context.Background(), "ws://localhost:3001")
		if err != nil {
			log.Println(errors.Join(errors.New("failed to dial proxied connection"), err))
			clientConn.Close()
			return
		}
		handleIncomingMessagesToProxy := func(clientConnection net.Conn, targetConnection net.Conn) {
			for {
				msg, op, err := wsutil.ReadClientData(clientConnection)
				if err != nil {
					clientConnection.Close()
					log.Println(errors.Join(err, errors.New("failed to read from client")))
					return
				}
				err = wsutil.WriteClientMessage(targetConnection, op, msg)
				if err != nil {
					log.Println(errors.Join(err, errors.New("failed to write to client")))
					return
				}
				if op == ws.OpClose {
					clientConnection.Close()
					log.Println("Client closed connection")
					return
				}
				log.Println("Proxied client message")
			}
		}
		proxySidecarServerToClient := func(serverConnection net.Conn, targetConnection net.Conn) {
			for {
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(serverConnection)
				if err != nil {
					serverConnection.Close()
					log.Println(errors.Join(err, errors.New("failed to read from server")))
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(targetConnection, op, msg)
				if err != nil {
					targetConnection.Close()
					log.Println(errors.Join(err, errors.New("failed to write to client")))
					return
				}
				if op == ws.OpClose {
					serverConnection.Close()
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
