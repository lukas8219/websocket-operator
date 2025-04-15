package main

import (
	"context"
	"flag"
	"log/slog"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"
	"os"
	"time"

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
	user                   string
	upstreamHost           string
	downstreamHost         string
	upstreamConn           net.Conn
	downstreamConn         net.Conn
	cancelUpstream         context.CancelFunc
	upstreamContext        context.Context
	upstreamCancellingChan chan int
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

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", false, "Debug mode")
	flag.Parse()
	logger.SetupLogger(*debug)
	InitializeLoadBalancer(*mode)
	slog.Info("Starting load balancer server", "port", *port, "mode", *mode)
	connections := make(map[string]*ConnectionTracker) //TODO: This could be a broadcast instead of a single recipient/connection
	go handleRebalanceLoop(connections)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("ws-user-id")
		if user == "" {
			slog.Error("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		connectionTracker := &ConnectionTracker{
			user:           user,
			downstreamHost: r.RemoteAddr,
		}
		connections[user] = connectionTracker

		connectionTracker.Debug("New connection")

		host := router.Route(user)
		connectionTracker.Debug("New connection")
		if host == "" {
			connectionTracker.Error("No host found for user")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		connectionTracker.upstreamHost = host
		connectionTracker.Debug("Upgrading HTTP connection")
		upgrader := ws.HTTPUpgrader{
			Header: http.Header{
				"x-ws-operator-proxy-instance": []string{os.Getenv("HOSTNAME")},
				"x-ws-operator-upstream-host":  []string{host},
			},
		}
		clientConn, _, _, err := upgrader.Upgrade(r, w)
		connectionTracker.downstreamConn = clientConn
		if err != nil {
			connectionTracker.Error("Failed to upgrade HTTP connection", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		connectionTracker.Debug("Upgraded HTTP connection")

		connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
		proxiedCon, err := connectToUpstreamAndProxyMessages(connectionTracker, user)
		if err != nil {
			connectionTracker.Error("Failed to connect to upstream", "error", err)
			connectionTracker.Close()
			return
		}
		connectionTracker.upstreamConn = proxiedCon
		connectionTracker.upstreamCancellingChan = make(chan int, 1)
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

func handleRebalanceLoop(connections map[string]*ConnectionTracker) {
	slog.Debug("Starting rebalance loop")
	for {
		select {
		case hosts := <-router.RebalanceRequests():
			slog.Debug("Received message to rebalance", "hosts", hosts)
			upstreamHostsToConnectionTracker := make(map[string]*ConnectionTracker)
			slog.Debug("Flat mapping ConnectionTracker to upstreamHosts", "connections", connections)
			for _, connectionTracker := range connections {
				upstreamHostsToConnectionTracker[connectionTracker.user] = connectionTracker
			}
			for _, affectedHost := range hosts {
				recipientId := affectedHost[0]
				newHost := affectedHost[1]
				connectionTracker := upstreamHostsToConnectionTracker[recipientId]
				if connectionTracker == nil {
					slog.Debug("No connection tracker found", "user", recipientId)
					continue
				}
				oldHost := connectionTracker.upstreamHost
				connectionTracker.Debug("Checking host", "user", recipientId)
				if connectionTracker.upstreamHost == newHost {
					connectionTracker.Debug("No need to rebalance")
					continue
				}
				connectionTracker.Debug("Waiting for upstream to cancel", "oldHost", oldHost)
				connectionTracker.cancelUpstream()
				connectionTracker.upstreamHost = newHost
				select {
				case <-connectionTracker.upstreamCancellingChan:
					connectionTracker.Debug("Successfully received cancellation signal")
				case <-time.After(5 * time.Second):
					connectionTracker.Error("Timeout waiting for upstream cancellation, proceeding anyway")
				}

				connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
				connectionTracker.Info("Rebalancing connection from", "old", oldHost, "new", newHost)
				_, err := connectToUpstreamAndProxyMessages(connectionTracker, connectionTracker.user)
				go handleIncomingMessagesToProxy(connectionTracker)
				delete(connections, recipientId)
				connections[recipientId] = connectionTracker
				if err != nil {
					connectionTracker.Error("Failed to reconnect to upstream", "error", err)
					connectionTracker.Close()
				}
			}
		case <-time.After(1 * time.Second):
		}
	}
}

func handleIncomingMessagesToProxy(connectionTracker *ConnectionTracker) {
	for {
		select {
		case <-connectionTracker.upstreamContext.Done():
			connectionTracker.Debug("Upstream context done")
			return
		default:
			clientConnection := connectionTracker.downstreamConn
			upstreamConnection := connectionTracker.upstreamConn
			msg, op, err := wsutil.ReadClientData(clientConnection)
			if err != nil {
				connectionTracker.Error("Failed to read from downstream", "error", err)
				return
			}
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				connectionTracker.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				connectionTracker.Debug("downstream server closed connection")
				return
			}
		}
	}
}

// TODO
func connectToUpstreamAndProxyMessages(connectionTracker *ConnectionTracker, user string) (net.Conn, error) {
	host := connectionTracker.upstreamHost
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"ws-user-id": []string{user},
		},
	}
	connectionTracker.Debug("Dialing upstream")
	proxiedConn, _, _, err := dialer.Dial(connectionTracker.upstreamContext, "ws://"+host)
	if err != nil {
		connectionTracker.Error("Failed to dial upstream", "error", err)
		return nil, err
	}
	connectionTracker.Debug("Connected to upstream")
	connectionTracker.upstreamConn = proxiedConn

	waitSignal := func() {
		connectionTracker.Debug("Closing upstream connection")
		select {
		case connectionTracker.upstreamCancellingChan <- 1:
			connectionTracker.Debug("Successfully signaled cancellation")
		default:
			connectionTracker.Debug("No one waiting for cancellation signal, skipping")
		}
	}

	//Missing DEFER
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-connectionTracker.upstreamContext.Done():
				connectionTracker.Debug("Upstream context done")
				proxiedConn.Close()
				return
			default:
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(proxiedConn)
				if err != nil {
					connectionTracker.Error("Failed to read from upstream", "error", err)
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(connectionTracker.downstreamConn, op, msg)
				if err != nil {
					connectionTracker.Error("Failed to write to downstream", "error", err)
					return
				}
				if op == ws.OpClose {
					connectionTracker.Debug("upstream server closed connection")
					return
				}
			}
		}
	}
	go proxySidecarServerToClient()
	return proxiedConn, nil
}
