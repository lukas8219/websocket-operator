package link

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/utils"
)

type Direct struct {
	header   string
	accept   Accept
	mu       sync.Mutex
	sessions map[*directSession]struct{}
	done     chan struct{}
	once     sync.Once
}

type directSession struct {
	Session
	owner *Direct
	once  sync.Once
	err   error
}

func NewDirect(header string, accept Accept) (*Direct, error) {
	if header == "" {
		return nil, errors.New("direct transport requires user header")
	}
	return &Direct{header: header, accept: accept, sessions: make(map[*directSession]struct{}), done: make(chan struct{})}, nil
}

func (s *directSession) Close() error {
	s.once.Do(func() {
		s.err = s.Session.Close()
		s.owner.mu.Lock()
		delete(s.owner.sessions, s)
		s.owner.mu.Unlock()
	})
	return s.err
}

func (d *Direct) track(session Session) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	select {
	case <-d.done:
		session.Close()
		return nil, errors.New("direct transport closed")
	default:
	}
	tracked := &directSession{Session: session, owner: d}
	d.sessions[tracked] = struct{}{}
	return tracked, nil
}

func (d *Direct) Dial(ctx context.Context, address, user string) (Session, error) {
	if user == "" {
		return nil, errors.New("session requires a user")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{d.header: []string{user}})}
	conn, buffered, _, err := dialer.Dial(ctx, "ws://"+address)
	if err != nil {
		return nil, err
	}
	return d.track(WebSocket(utils.WithBufferedReader(conn, buffered), ws.StateClientSide))
}

func (d *Direct) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-d.done:
			http.Error(w, "transport closed", http.StatusServiceUnavailable)
			return
		default:
		}
		user := r.Header.Get(d.header)
		if user == "" {
			http.Error(w, "missing user", http.StatusBadRequest)
			return
		}
		if d.accept == nil {
			http.Error(w, "sessions are not accepted here", http.StatusServiceUnavailable)
			return
		}
		conn, buffered, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		session, err := d.track(WebSocket(utils.WithBufferedReader(conn, buffered.Reader), ws.StateServerSide))
		if err != nil {
			return
		}
		if err := d.accept(user, session); err != nil {
			session.Close()
		}
	})
}

func (d *Direct) Done() <-chan struct{} { return d.done }
func (d *Direct) Close() error {
	d.once.Do(func() {
		d.mu.Lock()
		close(d.done)
		active := make([]*directSession, 0, len(d.sessions))
		for s := range d.sessions {
			active = append(active, s)
		}
		d.mu.Unlock()
		for _, s := range active {
			s.Close()
		}
	})
	return nil
}
