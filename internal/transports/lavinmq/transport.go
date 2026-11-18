// Package lavinmq implements the compiled AMQP transport. A successful Write
// means broker acceptance, not delivery to the recipient's application.
package lavinmq

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	amqp "github.com/rabbitmq/amqp091-go"
	"lukas8219/websocket-operator/internal/peer_discovery"
)

const MaxPayload = 16 << 20

var ErrUnavailable = errors.New("LavinMQ transport unavailable")

type Config struct {
	URL, Address, Prefix string
	Timeout              time.Duration
	Lookup               func([]byte) (peer_discovery.Peer, error)
	Deliver              func([]byte, ws.OpCode, []byte) error
}

type Transport struct {
	config    Config
	socket    net.Conn
	publisher *amqp.Channel
	confirmed <-chan amqp.Confirmation
	returned  <-chan amqp.Return
	gate      chan struct{}
	done      chan struct{}
	once      sync.Once
}

func queueName(prefix, address string) string {
	return fmt.Sprintf("%s.v1.%x", prefix, sha256.Sum256([]byte(address)))
}

func New(config Config) (_ *Transport, err error) {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	host, port, addressErr := net.SplitHostPort(config.Address)
	n, portErr := strconv.Atoi(port)
	_, urlErr := amqp.ParseURI(config.URL)
	if config.URL == "" || urlErr != nil || addressErr != nil || net.ParseIP(host) == nil || portErr != nil || n < 1 || n > 65535 || config.Timeout <= 0 || config.Prefix == "" || len(config.Prefix) > 187 || strings.HasPrefix(config.Prefix, "amq.") || config.Lookup == nil || config.Deliver == nil {
		return nil, errors.New("invalid LavinMQ configuration")
	}
	p := &Transport{config: config, gate: make(chan struct{}, 1), done: make(chan struct{})}
	var startup *time.Timer
	defer func() {
		if startup != nil {
			startup.Stop()
		}
		if err != nil {
			p.Close()
		}
	}()
	conn, err := amqp.DialConfig(config.URL, amqp.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		socket, dialErr := net.DialTimeout(network, address, config.Timeout)
		if dialErr == nil {
			p.socket = socket
			startup = time.AfterFunc(config.Timeout, func() { socket.Close() })
		}
		return socket, dialErr
	}})
	if err != nil {
		return nil, errors.New("LavinMQ connection failed")
	}
	receiver, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	p.publisher, err = conn.Channel()
	if err != nil {
		return nil, err
	}
	queue, err := receiver.QueueDeclare(queueName(config.Prefix, config.Address), false, true, true, false, amqp.Table{
		"x-message-ttl": int32(5000), "x-max-length": int32(256), "x-max-length-bytes": int32(MaxPayload), "x-overflow": "reject-publish",
	})
	if err != nil {
		return nil, err
	}
	if err = receiver.Qos(1, 0, false); err != nil {
		return nil, err
	}
	if err = p.publisher.Confirm(false); err != nil {
		return nil, err
	}
	p.confirmed = p.publisher.NotifyPublish(make(chan amqp.Confirmation, 1))
	p.returned = p.publisher.NotifyReturn(make(chan amqp.Return, 1))
	deliveries, err := receiver.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		return nil, err
	}
	closed := p.publisher.NotifyClose(make(chan *amqp.Error, 1))
	go func() { <-closed; p.Close() }()
	go p.consume(deliveries)
	return p, nil
}

// Done closes on broker/channel loss or an ambiguous publish timeout. The host
// must stop serving and restart this transport; messages are never auto-retried.
func (p *Transport) Done() <-chan struct{} { return p.done }

// Close is idempotent. A local delivery already in progress may finish afterward.
func (p *Transport) Close() error {
	p.once.Do(func() {
		close(p.done)
		if p.socket != nil {
			p.socket.Close()
		}
	})
	return nil
}

func (p *Transport) consume(deliveries <-chan amqp.Delivery) {
	defer p.Close()
	for message := range deliveries {
		select {
		case <-p.done:
			return
		default:
		}
		recipient, _ := message.Headers["recipient"].(string)
		opcode, _ := message.Headers["opcode"].(int32)
		op := ws.OpCode(opcode)
		var err error
		if recipient == "" || (opcode != int32(ws.OpText) && opcode != int32(ws.OpBinary)) || len(message.Body) > MaxPayload {
			err = errors.New("invalid LavinMQ envelope")
		} else {
			err = p.config.Deliver([]byte(recipient), op, message.Body)
		}
		if err != nil {
			slog.Warn("LavinMQ delivery dropped", "error", err)
			err = message.Reject(false)
		} else {
			err = message.Ack(false)
		}
		if err != nil {
			return
		}
	}
}

// Write borrows its inputs until return and serializes confirmations. A timeout
// while waiting for admission is harmless; once publishing starts it closes the
// connection, because the broker may already have accepted the message.
func (p *Transport) Write(recipient []byte, opcode ws.OpCode, payload []byte) error {
	return p.WriteContext(context.Background(), recipient, opcode, payload)
}

// WriteContext also bounds a session handshake by its caller's deadline.
func (p *Transport) WriteContext(parent context.Context, recipient []byte, opcode ws.OpCode, payload []byte) error {
	if len(recipient) == 0 || (opcode != ws.OpText && opcode != ws.OpBinary) || len(payload) > MaxPayload {
		return errors.New("invalid LavinMQ message")
	}
	ctx, cancel := context.WithTimeout(parent, p.config.Timeout)
	defer cancel()
	select {
	case <-p.done:
		return ErrUnavailable
	default:
	}
	peer, err := p.config.Lookup(recipient)
	if err != nil {
		return err
	}
	select {
	case p.gate <- struct{}{}:
	case <-p.done:
		return ErrUnavailable
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.gate }()
	select {
	case <-p.done:
		return ErrUnavailable
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// amqp091-go's PublishWithContext does not cancel blocked socket writes.
	stop := context.AfterFunc(ctx, func() { p.Close() })
	defer stop()
	err = p.publisher.Publish("", queueName(p.config.Prefix, peer.SocketAddres()), true, false, amqp.Publishing{
		ContentType: "application/octet-stream", Headers: amqp.Table{"recipient": string(recipient), "opcode": int32(opcode)}, Body: bytes.Clone(payload),
	})
	if err != nil {
		p.Close()
		return err
	}
	select {
	case confirmation, ok := <-p.confirmed:
		if !ok {
			p.Close()
			return ErrUnavailable
		}
		if !confirmation.Ack {
			return errors.New("LavinMQ rejected publish")
		}
		// AMQP sends a mandatory return before its corresponding confirmation.
		select {
		case returned, ok := <-p.returned:
			if !ok {
				return ErrUnavailable
			}
			return fmt.Errorf("LavinMQ target unavailable (%d)", returned.ReplyCode)
		default:
			return nil
		}
	case <-p.done:
		return ErrUnavailable
	case <-ctx.Done():
		p.Close()
		return ctx.Err()
	}
}
