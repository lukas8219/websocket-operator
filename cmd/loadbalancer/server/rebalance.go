package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	rslv "lukas8219/websocket-operator/internal/resolver"
	"time"
)

func handleRebalanceLoop(resolver rslv.Resolver, connections map[string]*connection.Connection) {
	slog.Debug("Starting rebalance loop")
	for _ = range resolver.VersionUpgradeChannel() {
		hosts, err := resolver.CurrentHosts()
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		slog.Debug("Received message to rebalance", "hosts", hosts)
		upstreamHostsToConnectionTracker := make(map[string]*connection.Connection, len(connections))
		for user, connectionTracker := range connections {
			newHost, err := resolver.Lookup([]byte(user))
			if err != nil {
				panic(err) //TODO
			}
			previousHost := connectionTracker.UpstreamHost()
			if previousHost == newHost.SocketAddres() {
				connectionTracker.Debug("No need to rebalance")
				continue
			}
			connectionTracker.Debug("Waiting for upstream to cancel", "previous", previousHost)
			connectionTracker.SwitchUpstreamHost(newHost.SocketAddres())

			select {
			case <-connectionTracker.UpstreamCancelChan():
				connectionTracker.Debug("Successfully received cancellation signal")
			case <-time.After(5 * time.Second):
				connectionTracker.Error("Timeout waiting for upstream cancellation, proceeding anyway")
			}
			//TODO: gut feeling here. either we move rebalance to the connection pkg or we re-design stuff
			//connectionTracker.UpstreamContext, connectionTracker.CancelUpstream = context.WithCancel(context.Background())
			connectionTracker.Info("Rebalancing connection from", "previous", previousHost, "new", newHost)
			//TODO: stopping down -> up could cause issues if this is mid read/write
			go connectionTracker.Handle()
			upstreamHostsToConnectionTracker[connectionTracker.User()] = connectionTracker
		}
	}
}
