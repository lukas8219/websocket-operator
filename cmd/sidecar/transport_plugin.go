package main

import (
	"github.com/gobwas/ws"
	"lukas8219/websocket-operator/internal/transports"
)

func localDelivery(connections *connectionRegistry) transports.Deliver {
	return func(recipient []byte, op ws.OpCode, payload []byte) error {
		connection := connections.get(string(recipient))
		if connection == nil {
			return transports.ErrRecipientNotConnected
		}
		return connection.deliverApplication(op, payload)
	}
}
