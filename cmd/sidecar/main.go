package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
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

	slog.Info("Starting server", "port", *port)
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
		slog.Debug("Request received", "method", r.Method, "path", r.URL.Path)
		if r.Method == http.MethodPost && r.URL.Path == "/message" {
			if !userBloomFilter.Contains(r.Header.Get("ws-user-id")) {
				slog.Debug("No recipient found in-memory", "user", r.Header.Get("ws-user-id"))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			userId := r.Header.Get("ws-user-id")
			conn := connections[userId]
			if conn == nil {
				slog.Debug("No connection found", "userId", userId)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				slog.Error("Failed to read request body", "error", err)
				return
			}

			opCode := ws.OpCode(body[0])
			message := body[1:]
			slog.Debug("Writing message to client", "userId", userId, "opCode", opCode, "message", string(message))
			err = wsutil.WriteClientMessage(conn, opCode, message)
			if err != nil {
				slog.Error("Failed to write WebSocket message", "error", err)
				return
			}
			return
		}
		user := r.Header.Get("ws-user-id")
		if user == "" {
			slog.Debug("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userBloomFilter.Add(user)
		slog.Info("New connection", "user", user)
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			slog.Error("Failed to upgrade HTTP connection", "error", err)
		}
		proxiedConn, _, _, err := ws.Dial(context.Background(), "ws://localhost:"+*targetPort)
		connections[user] = proxiedConn
		if err != nil {
			slog.Error("Failed to dial proxied connection", "error", err)
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
					slog.Error("Failed to read from client", "error", err)
					return
				}

				message := reflect.New(incomingMessageStruct).Interface()
				err = json.Unmarshal(msg, message)
				if err != nil {
					slog.Error("Failed to unmarshal message", "error", err)
					return
				}
				// Get the json.RawMessage as a byte slice
				rawBytes := reflect.ValueOf(message).Elem().FieldByName("RecipientId").Interface().(json.RawMessage)

				// If it's a JSON string (like "user123"), you need to unmarshal it
				var recipientIdString string
				if err := json.Unmarshal(rawBytes, &recipientIdString); err != nil {
					slog.Error("Failed to unmarshal recipientId", "error", err)
					return
				}

				// Now recipientIdString contains the actual string value
				slog.Debug("Message recipient", "recipientId", recipientIdString)
				if !userBloomFilter.Contains(recipientIdString) {
					slog.Debug("No recipient found in-memory. Routing message to the correct target.")
					err := proxy.SendProxiedMessage(recipientIdString, msg, op)
					if err != nil {
						slog.Error("Failed to route message", "error", err)
					}
					continue
				}

				//Might need to handle Close here

				recipientConnection := connections[recipientIdString]
				if recipientConnection == nil {
					slog.Debug("No connection found", "recipientId", recipientIdString)
					continue
				}
				err = wsutil.WriteClientMessage(recipientConnection, op, msg)
				if err != nil {
					slog.Error("Failed to write to client", "error", err)
					return
				}
				if op == ws.OpClose {
					slog.Info("Client closed connection")
					return
				}
			}
		}
		proxySidecarServerToClient := func(serverConnection net.Conn, targetConnection net.Conn) {
			defer closeConnections()
			for {
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(serverConnection)
				if err != nil {
					slog.Error("Failed to read from server", "error", err)
					return
				}

				//TODO: we might need to handle `recipientId` routing messages here also

				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(targetConnection, op, msg)
				if err != nil {
					slog.Error("Failed to write to client", "error", err)
					return
				}
				if op == ws.OpClose {
					slog.Info("Server closed connection")
					return
				}
			}
		}
		go proxySidecarServerToClient(proxiedConn, clientConn)
		go handleIncomingMessagesToProxy(clientConn, proxiedConn)
	}))
}
