package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"lukas8219/websocket-operator/cmd/sidecar/proxy"
	"lukas8219/websocket-operator/internal/logger"
	"net"
	"net/http"
	"os"
	"reflect"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type ConnectionTracker struct {
	user           string
	upstreamHost   string
	downstreamHost string
	upstreamConn   net.Conn
	downstreamConn net.Conn
}

func (c *ConnectionTracker) Info(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Info(message, args...)
	return c
}

func (c *ConnectionTracker) Error(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Error(message, args...)
	return c
}

func (c *ConnectionTracker) Debug(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).With("component", "connection-tracker").Debug(message, args...)
	return c
}

var incomingMessageStruct = reflect.StructOf([]reflect.StructField{
	{
		Name: "RecipientId",
		Type: reflect.TypeOf(json.RawMessage{}),
		Tag:  reflect.StructTag(`json:"recipientId"`), //TODO extract this to a external configurable pkg
	},
})

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	targetPort := flag.String("targetPort", "3001", "Port to target")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", false, "Debug mode")
	flag.Parse()
	logger.SetupLogger(*debug)
	proxy.InitializeProxy(*mode)
	slog.Info("Starting server", "port", *port)
	// Map to store active WebSocket connections
	// Key: user ID, Value: ConnectionTracker
	connections := make(map[string]*ConnectionTracker)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Request received", "method", r.Method, "path", r.URL.Path)
		if r.Method == http.MethodPost && r.URL.Path == "/message" {
			if connections[r.Header.Get("ws-user-id")] == nil {
				slog.Debug("No recipient found in-memory", "user", r.Header.Get("ws-user-id"))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			userId := r.Header.Get("ws-user-id")
			connectionTracker := connections[userId]
			if connectionTracker == nil {
				slog.Debug("No connection found", "userId", userId)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			//TODO use io.Pipe here
			body, err := io.ReadAll(r.Body)
			if err != nil {
				slog.Error("Failed to read request body", "error", err)
				return
			}

			opCode := ws.OpCode(body[0])
			message := body[1:]
			slog.Debug("Writing message to client", "userId", userId, "opCode", opCode, "message", string(message))
			err = wsutil.WriteClientMessage(connectionTracker.upstreamConn, opCode, message)
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
		slog := slog.With("recipientId", user)
		w.Header().Set("x-ws-operator-instance", os.Getenv("HOSTNAME"))
		slog.Info("New connection")
		slog.Debug("Upgrading HTTP connection")
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			slog.Error("Failed to upgrade HTTP connection", "error", err)
			return
		}
		slog.Debug("Dialing proxied connection")
		proxiedConn, _, _, err := ws.Dial(context.Background(), "ws://localhost:"+*targetPort)
		connectionTracker := &ConnectionTracker{
			user:           user,
			upstreamHost:   "localhost:" + *targetPort,
			downstreamHost: r.RemoteAddr,
			upstreamConn:   proxiedConn,
			downstreamConn: clientConn,
		}
		connections[user] = connectionTracker
		if err != nil {
			connectionTracker.Error("Failed to dial proxied connection", "error", err)
			clientConn.Close()
			return
		}
		//TODO no good here
		closeConnections := func() {
			connections[user] = nil
			connectionTracker.downstreamConn.Close()
			connectionTracker.upstreamConn.Close()
		}

		go proxySidecarServerToClient(closeConnections, connectionTracker)
		go handleIncomingMessagesToProxy(connections, closeConnections, connectionTracker)
	}))
}

func proxySidecarServerToClient(deferClose func(), connectionTracker *ConnectionTracker) {
	defer deferClose()
	for {
		//Read as client - from the server.
		msg, op, err := wsutil.ReadServerData(connectionTracker.upstreamConn)
		if err != nil {
			connectionTracker.Error("Failed to read from server", "error", err)
			return
		}

		//TODO: we might need to handle `recipientId` routing messages here also

		//Write as client - to the proxied connection
		err = wsutil.WriteServerMessage(connectionTracker.downstreamConn, op, msg)
		if err != nil {
			connectionTracker.Error("Failed to write to client", "error", err)
			return
		}
		if op == ws.OpClose {
			connectionTracker.Info("Server closed connection")
			return
		}
	}
}

// TODO functions is doing too much. Split into smaller modules
func handleIncomingMessagesToProxy(connections map[string]*ConnectionTracker, deferClose func(), connectionTracker *ConnectionTracker) {
	defer deferClose()
	for {
		msg, op, err := wsutil.ReadClientData(connectionTracker.downstreamConn)
		if err != nil {
			connectionTracker.Error("Failed to read from client", "error", err)
			return
		}

		message := reflect.New(incomingMessageStruct).Interface()
		err = json.Unmarshal(msg, message)
		if err != nil {
			connectionTracker.Error("Failed to unmarshal message", "error", err)
			return
		}
		// Get the json.RawMessage as a byte slice
		rawBytes := reflect.ValueOf(message).Elem().FieldByName("RecipientId").Interface().(json.RawMessage)

		// If it's a JSON string (like "user123"), you need to unmarshal it
		var recipientIdString string
		if err := json.Unmarshal(rawBytes, &recipientIdString); err != nil {
			connectionTracker.Error("Failed to unmarshal recipientId", "error", err)
			return
		}

		recipientConnection := connections[recipientIdString]

		slog.Debug("Message recipient", "recipientId", recipientIdString, "recipientConnection", recipientConnection)
		if recipientConnection == nil {
			slog.Debug("No recipient found in-memory. Routing message to the correct target.", "recipientId", recipientIdString)
			err := proxy.SendProxiedMessage(recipientIdString, msg, op)
			if err != nil {
				connectionTracker.Error("Failed to route message", "error", err)
			}
			continue
		}

		err = wsutil.WriteClientMessage(recipientConnection.upstreamConn, op, msg)
		if err != nil {
			connectionTracker.Error("Failed to write to client", "error", err, "recipientId", recipientIdString)
			return
		}
		if op == ws.OpClose {
			connectionTracker.Info("Client closed connection")
			return
		}
	}
}
