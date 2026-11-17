package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/utils"
	"net/http"
	"os"

	"github.com/gobwas/ws"
)

func createHandler(rslv *resolver.Resolver, connections *connectionRegistry, userHeader string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleConnection(*rslv, connections, userHeader, w, r)
	}
}

func handleConnection(rslv resolver.Resolver, connections *connectionRegistry, userHeader string, w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get(userHeader)
	if user == "" {
		slog.Error("No user id provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//TODO: we should only accept `NewConnection` already with client connection and host set.`
	//As only the `connection` pkg should alter it`.

	host, err := rslv.Lookup([]byte(user))
	if err != nil {
		slog.Error("No host found for user")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	slog.With("user", user).Debug("Upgrading HTTP connection")
	upgrader := ws.HTTPUpgrader{
		Header: http.Header{
			"x-ws-operator-proxy-instance": []string{os.Getenv("HOSTNAME")},
			"x-ws-operator-upstream-host":  []string{host.Hostname()},
		},
	}
	downstreamConn, buffered, _, err := upgrader.Upgrade(r, w)

	if err != nil {
		slog.With("user", user).Error("Failed to upgrade HTTP connection", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	downstreamConn = utils.WithBufferedReader(downstreamConn, buffered.Reader)
	proxiedConnection := connection.NewConnection(user, host.SocketAddres(), downstreamConn.RemoteAddr().String(), downstreamConn, userHeader)
	connections.store(proxiedConnection)

	proxiedConnection.Debug("New connection")
	go proxiedConnection.Handle()
}
