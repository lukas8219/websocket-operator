// Package routing extracts recipient keys without coupling dispatch to a wire format.
package routing

import (
	"encoding/json"
	"fmt"

	"github.com/gobwas/ws"
)

// Extractor implementations must be safe for concurrent calls and must not mutate
// payload. The result may alias payload and is valid until synchronous dispatch
// returns; retaining it beyond that point requires a copy.
type Extractor interface {
	Extract(opcode ws.OpCode, payload []byte) ([]byte, error)
}

type ExtractorFunc func(ws.OpCode, []byte) ([]byte, error)

func (f ExtractorFunc) Extract(op ws.OpCode, payload []byte) ([]byte, error) {
	return f(op, payload)
}

// Select resolves a compiled extension once at startup. JSON is the reserved default.
func Select(name string, plugins map[string]Extractor) (Extractor, error) {
	if name == "" || name == "json" {
		return JSON{}, nil
	}
	if plugin := plugins[name]; plugin != nil {
		return plugin, nil
	}
	return nil, fmt.Errorf("unknown routing plugin %q", name)
}

// JSON preserves the original RawMessage decoding rules, including duplicate,
// missing, null and escaped fields. JSON string decoding can allocate.
type JSON struct{}

func (JSON) Extract(_ ws.OpCode, payload []byte) ([]byte, error) {
	var envelope struct {
		Recipient json.RawMessage `json:"recipientId"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	var recipient string
	if err := json.Unmarshal(envelope.Recipient, &recipient); err != nil {
		return nil, err
	}
	return []byte(recipient), nil
}
