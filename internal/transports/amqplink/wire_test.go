package amqplink

import (
	"bytes"
	"testing"

	"github.com/gobwas/ws"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	for _, event := range []kind{open, ready, data, closeSession, heartbeat, route} {
		e := envelope{Kind: event, ID: "00112233445566778899aabbccddeeff", Source: "[::1]:8080", User: "recipient"}
		if event == route {
			e.ID = ""
		}
		if event == data || event == route {
			e.Opcode = ws.OpBinary
			e.Payload = []byte{0, 255, 1}
		}
		p, err := encode(e)
		if err != nil {
			t.Fatal(err)
		}
		result, err := decode(p)
		if err != nil || result.Kind != event || result.ID != e.ID || result.Source != e.Source || result.User != e.User || !bytes.Equal(result.Payload, e.Payload) {
			t.Fatalf("round trip %+v: %v", result, err)
		}
	}
}

func FuzzEnvelope(f *testing.F) {
	seed, _ := encode(envelope{Kind: data, ID: "00112233445566778899aabbccddeeff", Source: "127.0.0.1:3000", Opcode: ws.OpBinary, Payload: []byte{0, 255}})
	f.Add(seed)
	f.Add([]byte{1, 255, 255, 255, 255})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, input []byte) {
		e, err := decode(input)
		if err != nil {
			return
		}
		encoded, err := encode(e)
		if err != nil {
			t.Fatal(err)
		}
		again, err := decode(encoded)
		if err != nil || again.Kind != e.Kind || again.ID != e.ID || again.Source != e.Source || again.User != e.User || again.Opcode != e.Opcode || !bytes.Equal(again.Payload, e.Payload) {
			t.Fatal("accepted envelope does not round-trip")
		}
	})
}
