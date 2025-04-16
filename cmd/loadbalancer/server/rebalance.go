package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"lukas8219/websocket-operator/internal/route"
	"time"
)

func handleRebalanceLoop(router route.RouterImpl, connections map[string]*connection.Connection) {
	slog.Debug("Starting rebalance loop")
	for {
		select {
		case hosts := <-router.RebalanceRequests():
			slog.Debug("Received message to rebalance", "hosts", hosts)
			upstreamHostsToConnectionTracker := make(map[string]*connection.Connection)
			slog.Debug("Flat mapping ConnectionTracker to upstreamHosts", "connections", connections)
			for _, connectionTracker := range connections {
				upstreamHostsToConnectionTracker[connectionTracker.User()] = connectionTracker
			}
			for _, affectedHost := range hosts {
				recipientId := affectedHost[0]
				newHost := affectedHost[1]
				connectionTracker := upstreamHostsToConnectionTracker[recipientId]
				if connectionTracker == nil {
					slog.Debug("No connection tracker found", "user", recipientId)
					continue
				}
				oldHost := connectionTracker.UpstreamHost()
				if connectionTracker.UpstreamHost() == newHost {
					connectionTracker.Debug("No need to rebalance")
					continue
				}
				connectionTracker.Debug("Waiting for upstream to cancel", "oldHost", oldHost)
				connectionTracker.SwitchUpstreamHost(newHost)

				select {
				case <-connectionTracker.UpstreamCancelChan():
					connectionTracker.Debug("Successfully received cancellation signal")
				case <-time.After(5 * time.Second):
					connectionTracker.Error("Timeout waiting for upstream cancellation, proceeding anyway")
				}

				connections[recipientId] = connectionTracker
				//TODO: gut feeling here. either we move rebalance to the connection pkg or we re-design stuff
				//connectionTracker.UpstreamContext, connectionTracker.CancelUpstream = context.WithCancel(context.Background())
				connectionTracker.Info("Rebalancing connection from", "old", oldHost, "new", newHost)
				//TODO: stopping down -> up could cause issues if this is mid read/write
				go connectionTracker.Handle()
			}
		}
	}
}
