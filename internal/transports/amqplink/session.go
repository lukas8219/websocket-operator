package amqplink

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
)

var errOverflow = errors.New("transport session inbox is full")

type session struct {
	id, peer, user string
	out            func(context.Context, envelope) error
	remove         func(*session)
	budget         *budget
	incoming       chan envelope
	ready, done    chan struct{}
	readyOnce      sync.Once
	mu, writeMu    sync.Mutex
	closed         bool
	err            error
	lastSeen       atomic.Int64
}

func newSession(id, peer, user string, b *budget, out func(context.Context, envelope) error, remove func(*session)) *session {
	s := &session{id: id, peer: peer, user: user, budget: b, out: out, remove: remove, incoming: make(chan envelope, 16), ready: make(chan struct{}), done: make(chan struct{})}
	s.lastSeen.Store(time.Now().UnixNano())
	return s
}
func (s *session) activate() { s.readyOnce.Do(func() { close(s.ready) }) }
func (s *session) wait() error {
	select {
	case <-s.done:
		return s.err
	default:
	}
	select {
	case <-s.done:
		return s.err
	case <-s.ready:
		return nil
	}
}
func (s *session) Read() ([]byte, ws.OpCode, error) {
	if err := s.wait(); err != nil {
		return nil, 0, err
	}
	select {
	case <-s.done:
		return nil, 0, s.err
	case e := <-s.incoming:
		s.budget.release(len(e.Payload))
		return e.Payload, e.Opcode, nil
	}
}
func (s *session) Write(op ws.OpCode, payload []byte) error {
	if op != ws.OpText && op != ws.OpBinary {
		return errors.New("session requires a data message")
	}
	if err := s.wait(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return s.err
	default:
	}
	err := s.out(context.Background(), envelope{Kind: data, ID: s.id, Opcode: op, Payload: payload})
	if err != nil {
		s.stop(err)
	}
	return err
}

// The callback borrows its payload. Reserve global space before copying it.
func (s *session) offer(e envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.EOF
	}
	if !s.budget.take(len(e.Payload)) {
		return errOverflow
	}
	e.Payload = bytes.Clone(e.Payload)
	select {
	case s.incoming <- e:
		return nil
	default:
		s.budget.release(len(e.Payload))
		return errOverflow
	}
}

// stop releases local state without publishing. Endpoint loss must not spend
// one network timeout per session trying to notify an unreachable peer.
func (s *session) stop(err error) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.closed = true
	s.err = err
	close(s.done)
	for {
		select {
		case e := <-s.incoming:
			s.budget.release(len(e.Payload))
		default:
			s.mu.Unlock()
			s.remove(s)
			return true
		}
	}
}
func (s *session) Close() error {
	if !s.stop(io.EOF) {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.out(context.Background(), envelope{Kind: closeSession, ID: s.id})
}
