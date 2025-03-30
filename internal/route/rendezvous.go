package route

import (
	"context"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/dgryski/go-rendezvous"
)

type Router struct {
	rendezvous rendezvous.Rendezvous
}

func NewRouter() *Router {
	return &Router{
		rendezvous: *rendezvous.New([]string{}, func(s string) uint64 {
			var sum uint64
			for _, c := range s {
				sum += uint64(c)
			}
			return sum
		}),
	}
}

func (r *Router) Route(recipientId string) string {
	return r.rendezvous.Lookup(recipientId)
}

func (r *Router) Add(host []string) {
	if r.rendezvous.Lookup(host[0]) != "" {
		return
	}
	for _, host := range host {
		r.rendezvous.Add(host)
	}
}

func (r *Router) InitializeHosts() error {
	hosts, err := r.getCurrentHosts("sidecar")
	if err != nil {
		return err
	}
	r.Add(hosts)
	return nil
}

func createResolver() *net.Resolver {

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

func (r *Router) getCurrentHosts(service string) ([]string, error) {
	resolver := createResolver()
	log.Println("Getting random SRV host for service:", service)
	_, addrs, err := resolver.LookupSRV(context.Background(), "", "", service)
	log.Println("Addrs:", addrs)
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

	return addrPorts, nil
}

func (r *Router) GetRandomSRVHost(recipientId string, service string) string {
	return r.Route(recipientId)
}
