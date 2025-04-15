package server

import (
	"context"
	"log/slog"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"
	"os"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func createHandler(router route.RouterImpl, connections map[string]*ConnectionTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleConnection(router, connections, w, r)
	}
}

func handleConnection(router route.RouterImpl, connections map[string]*ConnectionTracker, w http.ResponseWriter, r *http.Request) {
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
