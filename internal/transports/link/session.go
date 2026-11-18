// Package link defines the bidirectional transport between load balancers and
// sidecars. Client and application WebSockets remain outside this boundary.
package link

import (
	"context"
	"net/http"

	"github.com/gobwas/ws"
)

// Session carries complete messages. Read transfers ownership of its payload;
// Write borrows its payload until return. One reader and concurrent writers are
// supported. Close is idempotent and unblocks Read and Write.
type Session interface {
	Read() ([]byte, ws.OpCode, error)
	Write(ws.OpCode, []byte) error
	Close() error
}

// Accept prepares the local application and starts its serving goroutines.
// It must return before those goroutines can use the session; an error rejects
// the session. The transport owns cleanup if setup or the peer fails.
type Accept func(user string, session Session) error

// Endpoint is the same interface in both binaries. Dial opens a session on a
// sidecar; Handler receives direct connections, while broker backends receive
// through their subscriptions. Done signals a terminal backend failure.
type Endpoint interface {
	Dial(context.Context, string, string) (Session, error)
	Handler() http.Handler
	Done() <-chan struct{}
	Close() error
}
