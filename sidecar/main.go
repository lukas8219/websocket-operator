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
		closeConnections := func() {
			log.Println("Closing connection")
			proxiedConn.Close()
			clientConn.Close()
		}
		proxyMessagesClientMessages := func(clientConnection net.Conn, targetConnection net.Conn) {
			defer closeConnections()
			for {
				msg, op, _ := wsutil.ReadClientData(clientConnection)
				err = wsutil.WriteServerMessage(targetConnection, op, msg)
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
		proxyServerMessages := func(serverConnection net.Conn, targetConnection net.Conn) {
			defer closeConnections()
			for {
				msg, op, _ := wsutil.ReadServerData(serverConnection)
				err = wsutil.WriteClientMessage(targetConnection, op, msg)
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
		go proxyMessagesClientMessages(proxiedConn, clientConn)
		go proxyServerMessages(clientConn, proxiedConn)
	}))
}
