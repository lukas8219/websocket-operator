package dns

import (
	"context"
	"log/slog"
	"lukas8219/websocket-operator/internal/rendezvous"
	"net"
	"os"
	"strconv"
	"time"
)

type DnsRouter struct {
	loadbalancer *rendezvous.Rendezvous
}

func (r *DnsRouter) Info(msg string, args ...any) {
	slog.With("component", "router").With("mode", "dns").Info(msg, args...)
}

func (r *DnsRouter) Debug(msg string, args ...any) {
	slog.With("component", "router").With("mode", "dns").Debug(msg, args...)
}

func (r *DnsRouter) Error(msg string, args ...any) {
	slog.With("component", "router").With("mode", "dns").Error(msg, args...)
}

func WithDns(loadbalancer *rendezvous.Rendezvous) *DnsRouter {
	return &DnsRouter{
		loadbalancer,
	}
}

func (r *DnsRouter) InitializeHosts() error {
	srvRecord := os.Getenv("WS_OPERATOR_SRV_DNS_RECORD")
	if srvRecord == "" {
		srvRecord = "ws-operator.local"
	}
	hosts, err := r.getCurrentHosts(srvRecord)
	if err != nil {
		return err
	}
	for _, host := range hosts {
		r.loadbalancer.Add(host)
	}
	return nil
}

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

func (r *DnsRouter) getCurrentHosts(service string) ([]string, error) {
	resolver := createResolver()
	slog.Debug("Getting random SRV host for service", "service", service)
	_, addrs, err := resolver.LookupSRV(context.Background(), "", "", service)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		return []string{}, nil
	}

	// Create a slice of tuples [addr,port] from the SRV records
	addrPorts := make([]string, len(addrs))
	for i, srv := range addrs {
		addr, err := resolver.LookupIP(context.Background(), "ip", srv.Target)
		if err != nil {
			return nil, err
		}
		addrPorts[i] = net.JoinHostPort(addr[0].String(), strconv.Itoa(int(srv.Port)))
	}
	slog.Debug("Found addresses", "addrs", addrPorts)
	return addrPorts, nil
}

func (r *DnsRouter) Route(recipientId string) string {
	return r.loadbalancer.Lookup(recipientId)
}

func (r *DnsRouter) RebalanceRequests() <-chan [][2]string {
	return nil
}
