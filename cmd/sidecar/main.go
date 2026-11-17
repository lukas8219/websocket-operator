package main

import (
	"context"
	"flag"
	"io"
	"log"
	"log/slog"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/routing"
	"lukas8219/websocket-operator/internal/transports"
	"lukas8219/websocket-operator/internal/utils"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type ConnectionTracker struct {
	applicationMu  sync.Mutex
	user           string
	upstreamHost   string
	downstreamHost string
	upstreamConn   net.Conn
	downstreamConn net.Conn
	transports.Transport
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

// Add compiled routing extensions here; selection happens before serving requests.
var routingPlugins = map[string]routing.Extractor{}

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	targetPort := flag.String("targetPort", "3001", "Port to target")
	// mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", false, "Debug mode")
	flag.Parse()
	userHeader, err := config.UserHeaderFromEnv()
	if err != nil {
		log.Fatalf("routing configuration: %v", err)
	}
	extractor, err := routing.Select(os.Getenv("WS_OPERATOR_ROUTING_PLUGIN"), routingPlugins)
	if err != nil {
		log.Fatalf("routing plugin: %v", err)
	}

	discovery, err := config.DiscoveryFromEnv()
	if err != nil {
		log.Fatalf("discovery configuration: %v", err)
	}
	logger.SetupLogger(*debug)
	//TODO move to config
	client, err := utils.NewKubernetesClient()
	if err != nil {
		log.Fatalf("initialize Kubernetes client: %v", err)
	}
	peerDiscovery := peer_discovery.NewKubernetes(peer_discovery.KubernetesPeerDiscoveryOptions{
		Clientset: client,
		Namespace: discovery.Namespace,
		Service:   discovery.Service,
	})
	resolver := resolver.New(peerDiscovery, consistent_hashing.NewJumpHash(peerDiscovery))
	go resolver.Init()
	connections := newConnectionRegistry()
	deliver := localDelivery(connections)
	transport, err := transports.Open(os.Getenv("WS_OPERATOR_TRANSPORT_PLUGIN"), transports.Options{
		Resolver: resolver, UserHeader: userHeader, Deliver: deliver,
	}, transportPlugins)
	if err != nil {
		log.Fatalf("transport plugin: %v", err)
	}
	if closer, ok := transport.(io.Closer); ok {
		defer closer.Close()
	}

	slog.Info("Starting server", "port", *port)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Request received", "method", r.Method, "path", r.URL.Path)
		if r.Method == http.MethodPost && r.URL.Path == "/message" {
			userId := r.Header.Get(userHeader)
			connectionTracker := connections.get(userId)
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

			if len(body) == 0 {
				http.Error(w, "message body must include an opcode", http.StatusBadRequest)
				return
			}

			opCode := ws.OpCode(body[0])
			message := body[1:]
			slog.Debug("Writing message to client", "userId", userId, "opCode", opCode, "message", string(message))
			err = deliver([]byte(userId), opCode, message)
			if err != nil {
				slog.Error("Failed to write WebSocket message", "error", err)
				return
			}
			return
		}
		user := r.Header.Get(userHeader)
		if user == "" {
			slog.Debug("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		slog := slog.With("recipientId", user)
		w.Header().Set("x-ws-operator-instance", os.Getenv("HOSTNAME"))
		slog.Debug("Dialing proxied connection")
		proxiedConn, upstreamReader, _, err := ws.Dial(context.Background(), "ws://localhost:"+*targetPort)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			slog.Error("Failed to establish upstream connection", "error", err)
			return
		}
		proxiedConn = utils.WithBufferedReader(proxiedConn, upstreamReader)
		slog.Debug("Connection established")
		slog.Debug("Upgrading HTTP connection")
		clientConn, buffered, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			slog.Error("Failed to upgrade HTTP connection", "error", err)
			return
		}
		clientConn = utils.WithBufferedReader(clientConn, buffered.Reader)
		connectionTracker := &ConnectionTracker{
			user:           user,
			upstreamHost:   "localhost:" + *targetPort,
			downstreamHost: r.RemoteAddr,
			upstreamConn:   proxiedConn,
			downstreamConn: clientConn,
			Transport:      transport,
		}
		connections.register(connectionTracker)
		var closeOnce sync.Once
		closeConnections := func() {
			closeOnce.Do(func() {
				connections.remove(connectionTracker)
				connectionTracker.downstreamConn.Close()
				connectionTracker.upstreamConn.Close()
			})
		}

		go proxySidecarServerToClient(closeConnections, connectionTracker)
		go handleIncomingMessagesToProxy(connections, closeConnections, connectionTracker, extractor)
	}))
}

func proxySidecarServerToClient(deferClose func(), connectionTracker *ConnectionTracker) {
	defer deferClose()
	for {
		//Read as client - from the server.
		msg, op, err := wsutil.ReadServerData(connectionTracker.applicationReader())
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
func handleIncomingMessagesToProxy(connections *connectionRegistry, deferClose func(), connectionTracker *ConnectionTracker, extractor routing.Extractor) {
	defer deferClose()
	for {
		msg, op, err := wsutil.ReadClientData(connectionTracker.downstreamConn)
		if err != nil {
			connectionTracker.Error("Failed to read from client", "error", err)
			return
		}

		recipient, err := extractor.Extract(op, msg)
		if err != nil {
			connectionTracker.Error("Failed to extract recipient", "error", err)
			return
		}
		recipientIdString := string(recipient)

		recipientConnection := connections.get(recipientIdString)

		slog.Debug("Message recipient", "recipientId", recipientIdString, "recipientConnection", recipientConnection)
		if recipientConnection == nil {
			slog.Debug("No recipient found in-memory. Routing message to the correct target.", "recipientId", recipientIdString)
			err := connectionTracker.Write(recipient, op, msg)
			if err != nil {
				connectionTracker.Error("Failed to route message", "error", err)
			}
			continue
		}

		err = recipientConnection.deliverApplication(op, msg)
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
