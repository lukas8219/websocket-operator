package transports

import "github.com/gobwas/ws"

// Transport delivers a routed message when its recipient is not connected locally.
// It does not establish client sessions or carry ordinary application replies.
// Transport borrows recipient and data until Write returns, without mutating or
// retaining them. Implementations must be safe for concurrent calls. Success means
// backend acceptance, not confirmation that the end user read the message.
type Transport interface {
	Write(Recipient []byte, OpCode ws.OpCode, Data []byte) error
}
