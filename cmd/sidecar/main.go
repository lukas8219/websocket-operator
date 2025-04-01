package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"lukas8219/websocket-operator/cmd/sidecar/collections"
	"lukas8219/websocket-operator/cmd/sidecar/proxy"
	"net"
	"net/http"
	"reflect"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func main() {
	userField := "recipientId"
	port := flag.String("port", "3000", "Port to listen on")
	targetPort := flag.String("targetPort", "3001", "Port to target")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	flag.Parse()
	proxy.InitializeProxy(*mode)

	log.Printf("Starting server on port %s", *port)
	//We might need to change for a Counting BloomFilter
	userBloomFilter := collections.New(1000000) // should we make this externally configurable?
	incomingMessageStruct := reflect.StructOf([]reflect.StructField{
		{
			Name: "RecipientId",
			Type: reflect.TypeOf(json.RawMessage{}),
			Tag:  reflect.StructTag(`json:"` + userField + `"`),
		},
	})
	// Map to store active WebSocket connections
	// Key: user ID, Value: net.Conn
	connections := make(map[string]net.Conn)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Request:", r.Method, r.URL.Path)
		if r.Method == http.MethodPost && r.URL.Path == "/message" {
			if !userBloomFilter.Contains(r.Header.Get("ws-user-id")) {
				log.Println("No recipient found in-memory", r.Header.Get("ws-user-id"))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			userId := r.Header.Get("ws-user-id")
			conn := connections[userId]
			if conn == nil {
				log.Println("No connection found for", userId)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("Failed to read request body: %v", err)
				return
			}

			opCode := ws.OpCode(body[0])
			message := body[1:]
			log.Println("Writing message to client", userId, opCode, string(body))
			err = wsutil.WriteClientMessage(conn, opCode, message)
			if err != nil {
				log.Printf("Failed to write WebSocket message: %v", err)
				return
			}
			return
		}
		user := r.Header.Get("ws-user-id")
		if user == "" {
			log.Println("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userBloomFilter.Add(user)
		log.Println("New connection from", user)
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println(err)
		}
		proxiedConn, _, _, err := ws.Dial(context.Background(), "ws://localhost:"+*targetPort)
		connections[user] = proxiedConn
		if err != nil {
			log.Println(errors.Join(errors.New("failed to dial proxied connection"), err))
			clientConn.Close()
			return
		}
		closeConnections := func() {
			userBloomFilter.Remove(user)
			connections[user] = nil
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

				message := reflect.New(incomingMessageStruct).Interface()
				err = json.Unmarshal(msg, message)
				if err != nil {
					log.Printf("Failed to unmarshal message: %v", err)
					return
				}
				// Get the json.RawMessage as a byte slice
				rawBytes := reflect.ValueOf(message).Elem().FieldByName("RecipientId").Interface().(json.RawMessage)

				// If it's a JSON string (like "user123"), you need to unmarshal it
				var recipientIdString string
				if err := json.Unmarshal(rawBytes, &recipientIdString); err != nil {
					log.Printf("Failed to unmarshal recipientId: %v", err)
					return
				}

				// Now recipientIdString contains the actual string value
				log.Println("RecipientId:", recipientIdString)
				if !userBloomFilter.Contains(recipientIdString) {
					log.Println("No recipient found in-memory. Routing message to the correct target.")
					err := proxy.SendProxiedMessage(recipientIdString, msg, op)
					if err != nil {
						log.Println(errors.Join(err, errors.New("failed to route message")))
					}
					continue
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
