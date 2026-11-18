package amqplink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/transports/link"
)

type hubOptions struct {
	accept                   link.Accept
	deliver                  func([]byte, ws.OpCode, []byte) error
	timeout, interval, lease time.Duration
}
type notice struct {
	peer  string
	event envelope
}
type hub struct {
	notices  chan notice
	options  hubOptions
	send     func(context.Context, string, envelope) error
	mu       sync.Mutex
	sessions map[string]*session
	budget   budget
	done     chan struct{}
	once     sync.Once
}

func newHub(options hubOptions, send func(context.Context, string, envelope) error) *hub {
	h := &hub{options: options, send: send, sessions: make(map[string]*session), budget: budget{limit: 16 << 20}, done: make(chan struct{})}
	h.notices = make(chan notice, 256)
	go h.notifications()
	go h.maintain()
	return h
}
func (h *hub) Done() <-chan struct{} { return h.done }
func (h *hub) Close() error {
	h.once.Do(func() {
		close(h.done)
		for _, s := range h.snapshot() {
			s.stop(io.ErrClosedPipe)
		}
	})
	return nil
}
func (h *hub) snapshot() []*session {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]*session, 0, len(h.sessions))
	for _, s := range h.sessions {
		result = append(result, s)
	}
	return result
}
func (h *hub) find(id string) *session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[id]
}
func (h *hub) allocate(id, peer, user string) (*session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.done:
		return nil, io.ErrClosedPipe
	default:
	}
	if len(h.sessions) >= 256 {
		return nil, errors.New("transport session limit reached")
	}
	if h.sessions[id] != nil {
		return nil, errors.New("session identity already exists")
	}
	s := newSession(id, peer, user, &h.budget, func(ctx context.Context, e envelope) error { return h.send(ctx, peer, e) }, func(s *session) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.sessions[s.id] == s {
			delete(h.sessions, s.id)
		}
	})
	h.sessions[id] = s
	return s, nil
}

func (h *hub) Dial(ctx context.Context, peer, user string) (link.Session, error) {
	if _, err := address([]byte(peer)); err != nil {
		return nil, err
	}
	if user == "" || len(user) > 512 {
		return nil, errors.New("invalid session user")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	s, err := h.allocate(hex.EncodeToString(id), peer, user)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, h.options.timeout)
	defer cancel()
	if err = h.send(ctx, peer, envelope{Kind: open, ID: s.id, User: user}); err != nil {
		s.stop(err)
		return nil, err
	}
	select {
	case <-s.ready:
		select {
		case <-s.done:
			return nil, s.err
		default:
			return s, nil
		}
	case <-s.done:
		return nil, s.err
	case <-ctx.Done():
		// Local removal is immediate; a pending remote open is bounded by its lease
		// even if this best-effort close cannot reach the receiver.
		s.stop(ctx.Err())
		h.notify(peer, envelope{Kind: closeSession, ID: s.id})
		return nil, ctx.Err()
	}
}

func (h *hub) receive(e envelope) error {
	if e.Kind == route {
		if h.options.deliver == nil {
			return errors.New("endpoint cannot deliver routed messages")
		}
		return h.options.deliver([]byte(e.User), e.Opcode, e.Payload)
	}
	if e.Kind == open {
		return h.accept(e)
	}
	s := h.find(e.ID)
	if s == nil || s.peer != e.Source {
		return nil
	}
	s.lastSeen.Store(time.Now().UnixNano())
	switch e.Kind {
	case ready:
		s.activate()
	case data:
		if err := s.offer(e); err != nil {
			s.Close()
			return err
		}
	case closeSession:
		s.stop(io.EOF)
	}
	return nil
}

func (h *hub) accept(e envelope) error {
	if h.options.accept == nil {
		return h.send(context.Background(), e.Source, envelope{Kind: closeSession, ID: e.ID})
	}
	if existing := h.find(e.ID); existing != nil {
		if existing.peer != e.Source || existing.user != e.User {
			return errors.New("conflicting session open")
		}
		select {
		case <-existing.ready:
			return h.send(context.Background(), e.Source, envelope{Kind: ready, ID: e.ID})
		default:
			return nil
		}
	}
	s, err := h.allocate(e.ID, e.Source, e.User)
	if err != nil {
		h.send(context.Background(), e.Source, envelope{Kind: closeSession, ID: e.ID})
		return err
	}
	go func() {
		if err := h.options.accept(e.User, s); err != nil {
			s.Close()
			return
		}
		select {
		case <-s.done:
			return
		default:
		}
		if err := h.send(context.Background(), s.peer, envelope{Kind: ready, ID: s.id}); err != nil {
			s.stop(err)
			return
		}
		s.activate()
	}()
	return nil
}

func (h *hub) maintain() {
	ticker := time.NewTicker(h.options.interval)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case now := <-ticker.C:
			for _, s := range h.snapshot() {
				limit := h.options.timeout
				active := false
				select {
				case <-s.ready:
					active = true
					limit = h.options.lease
				default:
				}
				if now.Sub(time.Unix(0, s.lastSeen.Load())) > limit {
					s.Close()
					continue
				}
				if active {
					if err := h.send(context.Background(), s.peer, envelope{Kind: heartbeat, ID: s.id}); err != nil {
						s.stop(err)
					}
				}
			}
		}
	}
}

func (h *hub) notify(peer string, e envelope) {
	select {
	case h.notices <- notice{peer, e}:
	default:
	}
}
func (h *hub) notifications() {
	for {
		select {
		case <-h.done:
			return
		case n := <-h.notices:
			h.send(context.Background(), n.peer, n.event)
		}
	}
}
