package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/routing"
	"lukas8219/websocket-operator/internal/transports"
	"lukas8219/websocket-operator/internal/transports/builtin"
	"lukas8219/websocket-operator/internal/transports/link"
	"lukas8219/websocket-operator/internal/utils"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type ConnectionTracker struct {
	applicationMu  sync.Mutex
	user           string
	upstreamHost   string
	downstreamHost string
	upstreamConn   net.Conn
	downstreamConn link.Session
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
	listenHost := flag.String("listenHost", "0.0.0.0", "Interface to listen on")
	port := flag.String("port", "3000", "Port to listen on")
	targetPort := flag.String("targetPort", "3001", "Port to target")
	// mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", false, "Debug mode")
	flag.Parse()
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
	transportConfig, err := builtin.FromEnv(*port)
	if err != nil {
		log.Fatalf("transport configuration: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var transport builtin.Backend
	initialized := make(chan struct{})
	accept := func(user string, session link.Session) error {
		select {
		case <-initialized:
		case <-ctx.Done():
			return ctx.Err()
		}
		dialContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		upstream, buffered, _, err := ws.Dial(dialContext, "ws://localhost:"+*targetPort)
		if err != nil {
			return err
		}
		tracker := &ConnectionTracker{user: user, upstreamHost: "localhost:" + *targetPort, downstreamHost: "transport", upstreamConn: utils.WithBufferedReader(upstream, buffered), downstreamConn: session, Transport: transport}
		connections.register(tracker)
		var once sync.Once
		closeConnections := func() {
			once.Do(func() {
				connections.remove(tracker)
				tracker.upstreamConn.Close()
				session.Close()
			})
		}
		go proxySidecarServerToClient(closeConnections, tracker)
		go handleIncomingMessagesToProxy(connections, closeConnections, tracker, extractor)
		return nil
	}
	transport, err = builtin.Open(transportConfig, builtin.Options{Resolver: resolver, Accept: accept, Deliver: localDelivery(connections)})
	if err != nil {
		log.Fatalf("transport plugin: %v", err)
	}
	close(initialized)
	handler := transport.Handler()
	server := &http.Server{Addr: net.JoinHostPort(*listenHost, *port), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ws-operator-instance", os.Getenv("HOSTNAME"))
		handler.ServeHTTP(w, r)
	})}
	slog.Info("Starting server", "port", *port)
	if err := link.Serve(ctx, transport, server); err != nil {
		slog.Info("Sidecar stopped", "error", err)
	}
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
		err = connectionTracker.downstreamConn.Write(op, msg)
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
		msg, op, err := connectionTracker.downstreamConn.Read()
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
