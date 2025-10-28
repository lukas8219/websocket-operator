package peer_discovery

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"
)

type DnsPeerDiscovery struct {
	srvRecord           string
	notificationChannel chan []Peer
	PeerDiscovery
}

func (r *DnsPeerDiscovery) Initialize() error {
	return nil
}

func NewDNS(srvRecord string) *DnsPeerDiscovery {
	return &DnsPeerDiscovery{
		srvRecord: srvRecord,
	}
}

func (r *DnsPeerDiscovery) NotificationChannel() chan []Peer {
	return r.notificationChannel
}

func (r *DnsPeerDiscovery) CurrentHosts() ([]Peer, error) {
	resolver := createResolver()
	slog.Debug("Getting random SRV host for service", "service", r.srvRecord)
	_, addrs, err := resolver.LookupSRV(context.Background(), "", "", r.srvRecord)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		return []Peer{}, nil
	}

	peers := make([]Peer, len(addrs))
	for i, srv := range addrs {
		addr, err := resolver.LookupIP(context.Background(), "ip", srv.Target)
		if err != nil {
			return nil, err
		}
		hostname := addr[0].String()
		port := strconv.Itoa(int(srv.Port))
		peers[i] = Peer{
			hostname,
			port,
		}
	}
	return peers, nil
}

// TODO review
func createResolver() *net.Resolver {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return &net.Resolver{}
	}
	// Create a custom resolver that first tries localhost:53 (for testing)
	// and falls back to the system resolver if that fails
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// First try localhost:53
			d := net.Dialer{}
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			slog.Debug("Looking for address on localhost:53", "address", address)
			conn, err := d.DialContext(ctx, "udp", "0.0.0.0:53")
			if err != nil {
				slog.Debug("Failed to connect to localhost:53, falling back to system resolver", "error", err)
				return d.DialContext(ctx, network, address)
			}
			return conn, nil
		},
	}

	return r
}
