package transports

import "github.com/gobwas/ws"

// Transport borrows recipient and data until Write returns, without mutating or
// retaining them. Implementations must be safe for concurrent calls. Success means
// backend acceptance, not confirmation that the end user read the message.
type Transport interface {
	Write(Recipient []byte, OpCode ws.OpCode, Data []byte) error
}
