// Package builtin is the single compiled transport registry used by both binaries.
package builtin

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/resolver"
	"lukas8219/websocket-operator/internal/transports"
	"lukas8219/websocket-operator/internal/transports/amqplink"
	"lukas8219/websocket-operator/internal/transports/link"
)

type Backend interface {
	link.Endpoint
	transports.Transport
}
type Config struct{ Name, URL, Address, Prefix, UserHeader string }
type Options struct {
	Resolver resolver.Resolver
	Accept   link.Accept
	Deliver  transports.Deliver
}
type Factory func(Config, Options) (Backend, error)

// Add compiled extensions here so both images contain the same implementation.
// Never mutate this registry after startup.
var Factories = map[string]Factory{
	"http": func(c Config, o Options) (Backend, error) {
		endpoint, err := link.NewDirect(c.UserHeader, o.Accept)
		if err != nil {
			return nil, err
		}
		sender := transports.NewHTTPTransport(o.Resolver, c.UserHeader)
		return &direct{Direct: endpoint, HttpTransport: &sender, header: c.UserHeader, deliver: o.Deliver}, nil
	},
	"lavinmq": func(c Config, o Options) (Backend, error) {
		return amqplink.New(amqplink.Config{URL: c.URL, Address: c.Address, Prefix: c.Prefix, Lookup: o.Resolver.Lookup, Accept: o.Accept, Deliver: o.Deliver})
	},
}

func FromEnv(port string) (Config, error) {
	discovery, err := config.DiscoveryFromEnv()
	if err != nil {
		return Config{}, err
	}
	header, err := config.UserHeaderFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{Name: os.Getenv("WS_OPERATOR_TRANSPORT_PLUGIN"), URL: os.Getenv("WS_OPERATOR_LAVINMQ_URL"), Address: net.JoinHostPort(os.Getenv("WS_OPERATOR_POD_IP"), port), Prefix: "ws-operator.link." + discovery.Namespace + "." + discovery.Service, UserHeader: header}, nil
}
func Open(c Config, o Options) (Backend, error) {
	if o.Resolver == nil {
		return nil, errors.New("transport requires a resolver")
	}
	if c.Name == "" {
		c.Name = "http"
	}
	factory := Factories[c.Name]
	if factory == nil {
		return nil, fmt.Errorf("unknown transport plugin %q", c.Name)
	}
	backend, err := factory(c, o)
	if err != nil {
		return nil, fmt.Errorf("transport plugin %q: %w", c.Name, err)
	}
	if backend == nil {
		return nil, errors.New("transport factory returned nil")
	}
	return backend, nil
}

type direct struct {
	*link.Direct
	*transports.HttpTransport
	header  string
	deliver transports.Deliver
}

func (d *direct) Handler() http.Handler {
	return transports.MessageHandler(d.header, d.deliver, d.Direct.Handler())
}
