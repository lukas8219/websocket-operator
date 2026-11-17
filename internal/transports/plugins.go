package transports

import (
	"errors"
	"fmt"

	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/resolver"
)

var ErrRecipientNotConnected = errors.New("recipient is not connected")

// Deliver synchronously hands a message to a local application connection.
// Inputs are borrowed and must not be retained or modified by the receiver.
type Deliver func(recipient []byte, opcode ws.OpCode, payload []byte) error

type Options struct {
	Resolver   resolver.Resolver
	UserHeader string
	Deliver    Deliver
}

// Factory is called once at startup. Backends must support concurrent Write
// calls; an optional io.Closer is called when the sidecar server returns.
type Factory func(Options) (Transport, error)

// Open selects a compiled backend. HTTP is the reserved default.
func Open(name string, options Options, plugins map[string]Factory) (Transport, error) {
	if name == "" || name == "http" {
		if options.Resolver == nil || options.UserHeader == "" {
			return nil, fmt.Errorf("HTTP transport requires resolver and user header")
		}
		backend := NewHTTPTransport(options.Resolver, options.UserHeader)
		return &backend, nil
	}
	factory := plugins[name]
	if factory == nil {
		return nil, fmt.Errorf("unknown transport plugin %q", name)
	}
	backend, err := factory(options)
	if err != nil {
		return nil, fmt.Errorf("transport plugin %q: %w", name, err)
	}
	if backend == nil {
		return nil, fmt.Errorf("transport plugin %q returned nil", name)
	}
	return backend, nil
}
