package amqplink

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/transports/lavinmq"
)

type kind uint8

const (
	open kind = iota + 1
	ready
	data
	closeSession
	heartbeat
	route
)
const maxHeader = 4096
const MaxPayload = lavinmq.MaxPayload - maxHeader - 5

// Metadata is encoded once, followed by raw payload bytes (no base64 encoding).
// Source identifies the node owning this session's other endpoint.
type envelope struct {
	Kind    kind      `json:"kind"`
	ID      string    `json:"session,omitempty"`
	Source  string    `json:"source"`
	User    string    `json:"user,omitempty"`
	Opcode  ws.OpCode `json:"opcode,omitempty"`
	Payload []byte    `json:"-"`
}

func address(value []byte) (peer_discovery.Peer, error) {
	host, port, err := net.SplitHostPort(string(value))
	n, numberErr := strconv.Atoi(port)
	if err != nil || numberErr != nil || net.ParseIP(host) == nil || n < 1 || n > 65535 || string(value) != net.JoinHostPort(host, strconv.Itoa(n)) {
		return peer_discovery.Peer{}, errors.New("invalid transport node address")
	}
	return peer_discovery.NewPeer(host, uint16(n)), nil
}

func (e envelope) validate() error {
	if _, err := address([]byte(e.Source)); err != nil {
		return err
	}
	if len(e.User) > 512 {
		return errors.New("session user exceeds 512 bytes")
	}
	if len(e.Payload) > MaxPayload {
		return errors.New("transport message too large")
	}
	if e.Kind < open || e.Kind > route {
		return errors.New("unknown transport event")
	}
	if e.Kind != route {
		id, err := hex.DecodeString(e.ID)
		if err != nil || len(id) != 16 {
			return errors.New("invalid session identity")
		}
	} else if e.ID != "" {
		return errors.New("routed message has a session identity")
	}
	if (e.Kind == open || e.Kind == route) && e.User == "" {
		return errors.New("missing session user")
	}
	if e.Kind == data || e.Kind == route {
		if e.Opcode != ws.OpText && e.Opcode != ws.OpBinary {
			return errors.New("invalid data opcode")
		}
	} else if len(e.Payload) != 0 {
		return errors.New("control event has a payload")
	}
	return nil
}

func encode(e envelope) ([]byte, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	header, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if len(header) > maxHeader {
		return nil, errors.New("transport metadata too large")
	}
	out := make([]byte, 5+len(header)+len(e.Payload))
	out[0] = 1
	binary.BigEndian.PutUint32(out[1:5], uint32(len(header)))
	copy(out[5:], header)
	copy(out[5+len(header):], e.Payload)
	return out, nil
}

// decode borrows its payload from the input; receivers must copy before enqueueing.
func decode(p []byte) (e envelope, err error) {
	if len(p) < 5 || p[0] != 1 || len(p) > lavinmq.MaxPayload {
		return e, errors.New("invalid transport envelope")
	}
	size := int(binary.BigEndian.Uint32(p[1:5]))
	if size == 0 || size > maxHeader || size > len(p)-5 {
		return e, errors.New("invalid transport metadata length")
	}
	if err = json.Unmarshal(p[5:5+size], &e); err != nil {
		return e, err
	}
	e.Payload = p[5+size:]
	return e, e.validate()
}
