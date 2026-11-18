package link

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type stoppingEndpoint struct {
	Endpoint
	done   chan struct{}
	closed atomic.Int32
}

func (e *stoppingEndpoint) Done() <-chan struct{} { return e.done }
func (e *stoppingEndpoint) Close() error          { e.closed.Add(1); return nil }

func TestServeClosesEveryListener(t *testing.T) {
	for _, failure := range []string{"cancel", "backend", "listener"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			endpoint := &stoppingEndpoint{done: make(chan struct{})}
			addresses := make(chan string, 2)
			servers := make([]*http.Server, 2)
			for i := range servers {
				servers[i] = &http.Server{Addr: "127.0.0.1:0", BaseContext: func(listener net.Listener) context.Context {
					addresses <- listener.Addr().String()
					return ctx
				}}
			}
			ended := make(chan error, 1)
			go func() { ended <- Serve(ctx, endpoint, servers...) }()
			var bound []string
			for range servers {
				select {
				case addr := <-addresses:
					bound = append(bound, addr)
				case <-time.After(time.Second):
					t.Fatal("listener did not start")
				}
			}
			switch failure {
			case "cancel":
				cancel()
			case "backend":
				close(endpoint.done)
			case "listener":
				servers[0].Close()
			}
			select {
			case err := <-ended:
				if err == nil || endpoint.closed.Load() != 1 {
					t.Fatalf("shutdown: %v, closes=%d", err, endpoint.closed.Load())
				}
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
			}
			for _, addr := range bound {
				if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
					conn.Close()
					t.Errorf("listener %s survived shutdown", addr)
				}
			}
		})
	}
}
