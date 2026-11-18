package link

import (
	"context"
	"errors"
	"net/http"
)

// Serve closes every listener and transport when a backend fails, the process is
// stopped, or a listener exits. This includes the load balancer's discovery port.
func Serve(ctx context.Context, endpoint Endpoint, servers ...*http.Server) error {
	defer endpoint.Close()
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()
	ended := make(chan error, len(servers))
	for _, server := range servers {
		go func(server *http.Server) { ended <- server.ListenAndServe() }(server)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-endpoint.Done():
		return errors.New("transport stopped")
	case err := <-ended:
		return err
	}
}
