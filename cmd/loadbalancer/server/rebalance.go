package server

import (
	"log/slog"
	rslv "lukas8219/websocket-operator/internal/resolver"
	"time"
)

func handleRebalanceLoop(resolver rslv.Resolver, connections *connectionRegistry) {
	slog.Info("Starting rebalance loop")
	for _ = range resolver.VersionUpgradeChannel() {
		for _, connectionTracker := range connections.snapshot() {
			newHost, err := resolver.Lookup([]byte(connectionTracker.User()))
			if err != nil {
				connectionTracker.Error("No peer available; closing connection", "error", err)
				connectionTracker.Close()
				connections.remove(connectionTracker)
				continue
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
		}
	}
}
