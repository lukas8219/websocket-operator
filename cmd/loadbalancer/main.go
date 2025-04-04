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
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).Info(message, args...)
	return c
}

func (c *ConnectionTracker) Error(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).Error(message, args...)
	return c
}

func (c *ConnectionTracker) Debug(message string, args ...any) *ConnectionTracker {
	slog.With("user", c.user).With("upstreamHost", c.upstreamHost).With("downstreamHost", c.downstreamHost).Debug(message, args...)
	return c
}

func main() {
	logger.SetupLogger("loadbalancer")
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	flag.Parse()
	InitializeLoadBalancer(*mode)
	slog.Info("Starting load balancer server", "port", *port, "mode", *mode)
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
		connectionTracker.Info("Connected to upstream")

		router.OnHostRebalance(func(hosts [][2]string) error {
			connectionTracker.Debug("Triggered OnHostRebalance callback returning hosts", "hosts", hosts)
			for _, affectedHost := range hosts {
				oldHost := affectedHost[0]
				newHost := affectedHost[1]
				connectionTracker.Debug("Checking host", "oldHost", oldHost)
				if connectionTracker.upstreamHost == oldHost {
					connectionTracker.Info("Rebalancing connection from", "old", oldHost, "new", newHost)
					connectionTracker.cancelUpstream()
					connectionTracker.upstreamHost = newHost

					connectionTracker.Debug("Waiting for upstream to cancel", "old", oldHost)
					select {
					case <-connectionTracker.upstreamCancellingChan:
						connectionTracker.Debug("Successfully received cancellation signal")
					case <-time.After(5 * time.Second):
						connectionTracker.Error("Timeout waiting for upstream cancellation, proceeding anyway")
					}

					connectionTracker.Debug("Rebalancing host", "host", host, "to", newHost)
					connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())

					_, err = connectToUpstreamAndProxyMessages(connectionTracker, user)
					if err != nil {
						connectionTracker.Error("Failed to reconnect to upstream", "error", err)
						connectionTracker.Close()
						return err
					}
					break
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
	for {
		clientConnection := connectionTracker.downstreamConn
		upstreamConnection := connectionTracker.upstreamConn
		msg, op, err := wsutil.ReadClientData(clientConnection)
		if err != nil {
			connectionTracker.Error("Failed to read from downstream", "error", err)
			return
		}
		select {
		case <-connectionTracker.upstreamContext.Done():
			connectionTracker.Debug("Upstream context done")
			return
		default:
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
