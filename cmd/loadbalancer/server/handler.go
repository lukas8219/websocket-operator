package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/route"
	"net/http"
	"os"

	"github.com/gobwas/ws"
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

	host := router.Route(user)
	slog.With("user", user).Debug("New connection")
	if host == "" {
		slog.Error("No host found for user")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	slog.With("user", user).Debug("Upgrading HTTP connection")
	upgrader := ws.HTTPUpgrader{
		Header: http.Header{
			"x-ws-operator-proxy-instance": []string{os.Getenv("HOSTNAME")},
			"x-ws-operator-upstream-host":  []string{host},
		},
	}
	downstreamConn, _, _, err := upgrader.Upgrade(r, w)

	if err != nil {
		slog.With("user", user).Error("Failed to upgrade HTTP connection", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxiedConnection := connection.NewConnection(user, host, downstreamConn.RemoteAddr().String(), downstreamConn)
	connections[user] = proxiedConnection

	proxiedConnection.Debug("New connection")
	go proxiedConnection.Handle()
}
