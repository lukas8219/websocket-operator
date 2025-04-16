package server

import (
	"context"
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/route"
	"net/http"
	"os"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func createHandler(router route.RouterImpl, connections map[string]*connection.Connection) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleConnection(router, connections, w, r)
	}
}

func handleConnection(router route.RouterImpl, connections map[string]*connection.Connection, w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("ws-user-id")
	if user == "" {
		slog.Error("No user id provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//TODO: we should only accept `NewConnection` already with client connection and host set.`
	//As only the `connection` pkg should alter it`.
	proxiedConnection := connection.NewConnection(user, r.RemoteAddr, os.Getenv("HOSTNAME"))
	connections[user] = proxiedConnection

	proxiedConnection.Debug("New connection")

	host := router.Route(user)
	proxiedConnection.Debug("New connection")
	if host == "" {
		proxiedConnection.Error("No host found for user")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	proxiedConnection.SetUpstreamHost(host)
	proxiedConnection.Debug("Upgrading HTTP connection")
	upgrader := ws.HTTPUpgrader{
		Header: http.Header{
			"x-ws-operator-proxy-instance": []string{os.Getenv("HOSTNAME")},
			"x-ws-operator-upstream-host":  []string{host},
		},
	}
	clientConn, _, _, err := upgrader.Upgrade(r, w)
	proxiedConnection.DownstreamConn = clientConn
	if err != nil {
		proxiedConnection.Error("Failed to upgrade HTTP connection", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	proxiedConnection.Debug("Upgraded HTTP connection")

	proxiedConnection.UpstreamContext, proxiedConnection.CancelUpstream = context.WithCancel(context.Background())
	proxiedCon, err := proxiedConnection.ProxyDownstreamToUpstream()
	if err != nil {
		proxiedConnection.Error("Failed to connect to upstream", "error", err)
		proxiedConnection.Close()
		return
	}
	proxiedConnection.UpstreamConn = proxiedCon
	proxiedConnection.UpstreamCancellingChan = make(chan int, 1)
	go handleIncomingMessagesToProxy(proxiedConnection)
}

func handleIncomingMessagesToProxy(proxiedConnection *connection.ProxiedConnectionImpl) {
	for {
		select {
		case <-proxiedConnection.UpstreamContext.Done():
			proxiedConnection.Debug("Upstream context done")
			return
		default:
			clientConnection := proxiedConnection.DownstreamConn
			upstreamConnection := proxiedConnection.UpstreamConn
			msg, op, err := wsutil.ReadClientData(clientConnection)
			if err != nil {
				proxiedConnection.Error("Failed to read from downstream", "error", err)
				return
			}
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				proxiedConnection.Error("Failed to write to upstream", "error", err)
				return
			}
			if op == ws.OpClose {
				proxiedConnection.Debug("downstream server closed connection")
				return
			}
		}
	}
}
