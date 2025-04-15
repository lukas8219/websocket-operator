package server

import (
	"context"
	"log/slog"
	"lukas8219/websocket-operator/internal/route"
	"time"
)

func handleRebalanceLoop(router route.RouterImpl, connections map[string]*ConnectionTracker) {
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
		}
	}
}
