package transports

import "github.com/gobwas/ws"

type Transport interface {
	Write(Recipient []byte, OpCode ws.OpCode, Data []byte) error
}
