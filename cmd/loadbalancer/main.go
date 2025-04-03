package main

import (
	"context"
	"flag"
	"log/slog"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"
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
	upstreamHost           string
	upstreamConn           net.Conn
	downstreamConn         net.Conn
	cancelUpstream         context.CancelFunc
	upstreamContext        context.Context
	upstreamCancellingChan chan int
}

func main() {
	logger.SetupLogger("loadbalancer")
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	flag.Parse()
	InitializeLoadBalancer(*mode)
	slog.Info("Starting load balancer server at port" + *port)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("ws-user-id")
		if user == "" {
			slog.Error("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		slog.Debug("New connection from", "user", user)

		host := router.Route(user)
		if host == "" {
			slog.Error("No host found for user", "user", user)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			slog.Error("Failed to upgrade HTTP connection", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		connectionTracker := &ConnectionTracker{
			upstreamHost:   host,
			downstreamConn: clientConn,
		}

		connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
		proxiedCon, err := connectToUpstreamAndProxyMessages(connectionTracker, user)
		connectionTracker.upstreamConn = proxiedCon
		connectionTracker.upstreamCancellingChan = make(chan int, 1)

		router.OnHostRebalance(func(hosts [][2]string) error {
			slog.Debug("OnHostRebalance", "hosts", hosts)
			for _, affectedHost := range hosts {
				oldHost := affectedHost[0]
				newHost := affectedHost[1]
				slog.Debug("Checking host", "oldHost", oldHost, "versus", connectionTracker.upstreamHost)
				if connectionTracker.upstreamHost == oldHost {
					slog.Debug("Rebalancing connection from", "oldHost", oldHost, "to", newHost)
					connectionTracker.cancelUpstream()
					connectionTracker.upstreamHost = newHost

					slog.Debug("Waiting for upstream to cancel", "oldHost", oldHost)
					select {
					case <-connectionTracker.upstreamCancellingChan:
						slog.Debug("Successfully received cancellation signal")
					case <-time.After(5 * time.Second):
						slog.Debug("Timeout waiting for upstream cancellation, proceeding anyway")
					}

					slog.Debug("Rebalancing host", "host", host, "to", newHost)
					connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
					connectionTracker.upstreamCancellingChan = make(chan int, 1)
					_, err = connectToUpstreamAndProxyMessages(connectionTracker, user)
					if err != nil {
						slog.Error("Failed to reconnect to upstream", "error", err)
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
			slog.Error("Failed to read from downstream", "error", err)
			return
		}
		select {
		case <-connectionTracker.upstreamContext.Done():
			slog.Debug("Upstream context done")
			return
		default:
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				slog.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				slog.Debug("downstream server closed connection")
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
	slog.Debug("Dialing upstream", "host", host)
	proxiedConn, _, _, err := dialer.Dial(connectionTracker.upstreamContext, "ws://"+host)
	if err != nil {
		slog.Error("Failed to dial upstream", "error", err)
		return nil, err
	}
	slog.Debug("Connected to upstream", "host", host)
	connectionTracker.upstreamConn = proxiedConn

	waitSignal := func() {
		slog.Debug("Closing upstream connection", "host", host)
		select {
		case connectionTracker.upstreamCancellingChan <- 1:
			slog.Debug("Successfully signaled cancellation")
		default:
			slog.Debug("No one waiting for cancellation signal, skipping")
		}
	}

	//Missing DEFER
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-connectionTracker.upstreamContext.Done():
				slog.Debug("Upstream context done", "host", host)
				proxiedConn.Close()
				return
			default:
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(proxiedConn)
				if err != nil {
					slog.Error("Failed to read from upstream", "error", err)
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(connectionTracker.downstreamConn, op, msg)
				if err != nil {
					slog.Error("Failed to write to downstream", "error", err)
					return
				}
				if op == ws.OpClose {
					slog.Debug("upstream server closed connection", "host", host)
					return
				}
			}
		}
	}
	go proxySidecarServerToClient()
	return proxiedConn, nil
}
