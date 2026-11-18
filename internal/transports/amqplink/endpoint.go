package amqplink

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/transports/lavinmq"
	"lukas8219/websocket-operator/internal/transports/link"
)

type Config struct {
	URL, Address, Prefix      string
	Lookup                    func([]byte) (peer_discovery.Peer, error)
	Accept                    link.Accept
	Deliver                   func([]byte, ws.OpCode, []byte) error
	Timeout, Heartbeat, Lease time.Duration
}

type Endpoint struct {
	*hub
	config Config
	wire   *lavinmq.Transport
	routes chan envelope
}

func New(config Config) (*Endpoint, error) {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Heartbeat == 0 {
		config.Heartbeat = 5 * time.Second
	}
	if config.Lease == 0 {
		config.Lease = 20 * time.Second
	}
	if config.Timeout <= 0 || config.Heartbeat <= 0 || config.Lease < 2*config.Heartbeat {
		return nil, errors.New("invalid transport timeouts")
	}
	if _, err := address([]byte(config.Address)); err != nil {
		return nil, err
	}
	endpoint := &Endpoint{config: config, routes: make(chan envelope, 16)}
	initialized := make(chan struct{})
	endpoint.hub = newHub(hubOptions{accept: config.Accept, deliver: endpoint.route, timeout: config.Timeout, interval: config.Heartbeat, lease: config.Lease}, func(ctx context.Context, peer string, e envelope) error {
		select {
		case <-initialized:
		case <-endpoint.done:
			return io.ErrClosedPipe
		}
		e.Source = config.Address
		payload, err := encode(e)
		if err != nil {
			return err
		}
		return endpoint.wire.WriteContext(ctx, []byte(peer), ws.OpBinary, payload)
	})
	wire, err := lavinmq.New(lavinmq.Config{URL: config.URL, Address: config.Address, Prefix: config.Prefix, Timeout: config.Timeout, Lookup: address, Deliver: func(recipient []byte, op ws.OpCode, body []byte) error {
		if string(recipient) != config.Address || op != ws.OpBinary {
			return errors.New("invalid link destination")
		}
		event, err := decode(body)
		if err != nil {
			return err
		}
		return endpoint.receive(event)
	}})
	if err != nil {
		endpoint.hub.Close()
		return nil, err
	}
	endpoint.wire = wire
	close(initialized)
	go endpoint.deliverRoutes()
	go func() { <-wire.Done(); endpoint.Close() }()
	return endpoint, nil
}

func (e *Endpoint) Close() error {
	e.hub.Close()
	return e.wire.Close()
}
func (e *Endpoint) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "this endpoint requires the LavinMQ session plugin", http.StatusServiceUnavailable)
	})
}
func (e *Endpoint) Write(recipient []byte, op ws.OpCode, payload []byte) error {
	if e.config.Lookup == nil {
		return errors.New("recipient routing unavailable")
	}
	peer, err := e.config.Lookup(recipient)
	if err != nil {
		return err
	}
	return e.send(context.Background(), peer.SocketAddres(), envelope{Kind: route, User: string(recipient), Opcode: op, Payload: payload})
}

// Routed application writes run outside the AMQP consumer so a slow application
// cannot prevent session heartbeats and close messages from being processed.
func (e *Endpoint) route(user []byte, op ws.OpCode, body []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case <-e.done:
		return io.ErrClosedPipe
	default:
	}
	if e.config.Deliver == nil {
		return errors.New("no local application delivery")
	}
	if !e.budget.take(len(body)) {
		return errOverflow
	}
	message := envelope{User: string(user), Opcode: op, Payload: bytes.Clone(body)}
	select {
	case e.routes <- message:
		return nil
	default:
		e.budget.release(len(body))
		return errOverflow
	}
}
func (e *Endpoint) deliverRoutes() {
	defer func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for {
			select {
			case m := <-e.routes:
				e.budget.release(len(m.Payload))
			default:
				return
			}
		}
	}()
	for {
		select {
		case <-e.done:
			return
		default:
		}
		select {
		case <-e.done:
			return
		case m := <-e.routes:
			if err := e.config.Deliver([]byte(m.User), m.Opcode, m.Payload); err != nil {
				slog.Warn("Routed session message dropped", "error", err)
			}
			e.budget.release(len(m.Payload))
		}
	}
}
