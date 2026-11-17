## Compiled routing plugins

`WS_OPERATOR_ROUTING_PLUGIN` selects a compiled `routing.Extractor`; unset or
`json` keeps the existing recipientId JSON contract. Unknown names fail startup.
Implement `Extract(opcode, payload)`, add the implementation to `routingPlugins`
in `main.go`, and build your sidecar image. The selector runs once at startup.
The user-ID handshake header and peer hashing remain unchanged.

An extractor may return a slice of the original payload for a binary protocol.
It must support concurrent calls, must not mutate input, and must copy anything
it retains beyond synchronous dispatch. Dispatch forwards the original message.
The JSON implementation still allocates to decode escaped strings; registry/header
key conversion can also allocate. The interface adds a Go method dispatch, so this
is not a promise of zero-cost or end-to-end zero-copy execution. Extensions run with
the sidecar's privileges and panic behavior; Lua/C/Wasm loading is not included.

Validate an extension with malformed messages, concurrent calls and allocation
benchmarks. The default parser has differential fuzz coverage against the old
JSON decoder, including missing/null/duplicate fields and escape sequences.
