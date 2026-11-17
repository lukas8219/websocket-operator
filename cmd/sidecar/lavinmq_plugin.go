package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"lukas8219/websocket-operator/internal/config"
	"lukas8219/websocket-operator/internal/transports"
	"lukas8219/websocket-operator/internal/transports/lavinmq"
)

func lavinMQFactory(address string, discovery config.Discovery) transports.Factory {
	return func(options transports.Options) (transports.Transport, error) {
		if options.Resolver == nil {
			return nil, errors.New("LavinMQ requires a resolver")
		}
		return lavinmq.New(lavinmq.Config{
			URL: os.Getenv("WS_OPERATOR_LAVINMQ_URL"), Address: address,
			Prefix: discovery.Namespace + "." + discovery.Service,
			Lookup: options.Resolver.Lookup, Deliver: options.Deliver,
		})
	}
}

// A terminal plugin failure ends the process so Kubernetes restarts the sidecar.
// Close also runs on SIGTERM and listener errors. HTTP needs no Done method.
func serveTransport(ctx context.Context, server *http.Server, transport transports.Transport) error {
	if closer, ok := transport.(io.Closer); ok {
		defer closer.Close()
	}
	var failed <-chan struct{}
	if notifier, ok := transport.(interface{ Done() <-chan struct{} }); ok {
		failed = notifier.Done()
	}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-failed:
			slog.Error("Transport stopped; exiting sidecar for restart")
		case <-ctx.Done():
		case <-stopped:
			return
		}
		server.Close()
	}()
	return server.ListenAndServe()
}

func serveSidecar(address string, transport transports.Transport, handler http.Handler) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: address, Handler: handler}
	if err := serveTransport(ctx, server, transport); err != nil && err != http.ErrServerClosed {
		slog.Error("Sidecar server stopped", "error", err)
	}
}
