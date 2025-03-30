package dns

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/dgryski/go-rendezvous"
)

type DnsRouter struct {
	loadbalancer *rendezvous.Rendezvous
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
			log.Println("Looking for " + address + " on localhost:53")
			conn, err := d.DialContext(ctx, "udp", "0.0.0.0:53")
			if err != nil {
				log.Println("Failed to connect to localhost:53, falling back to system resolver:", err)
				return d.DialContext(ctx, network, address)
			}
			return conn, nil
		},
	}

	return r
}

func (r *DnsRouter) getCurrentHosts(service string) ([]string, error) {
	resolver := createResolver()
	log.Println("Getting random SRV host for service:", service)
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
	log.Println("Addrs:", addrPorts)
	return addrPorts, nil
}

func (r *DnsRouter) Route(recipientId string) string {
	return r.loadbalancer.Lookup(recipientId)
}
