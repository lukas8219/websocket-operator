## Compiled transport plugins

`WS_OPERATOR_TRANSPORT_PLUGIN` selects a compiled backend; unset or `http` keeps
HTTP delivery. Add a `transports.Factory` to `transportPlugins` and rebuild the
sidecar image. Unknown names or factory errors stop startup. HTTP is reserved.
Factories receive discovery routing, the configured recipient header, and a
synchronous `Deliver` callback for incoming messages to local applications.
The internal HTTP endpoint remains available and uses that same callback.

A backend implements concurrent-safe `Transport.Write`. Recipient/payload slices
are borrowed only until the method returns: asynchronous publishers must copy or
wait until their client stops reading. The delivery callback follows the same
borrowing rule and reports `ErrRecipientNotConnected` for an absent recipient.
An optional `io.Closer` runs when the server returns; this change does not add
signal-driven graceful shutdown or broker reconnection. Plugins own receiver
lifecycle, timeouts, credentials, and error/acknowledgment policy. Successful Write
means backend acceptance, not end-user acknowledgment or exactly-once delivery.

Selection happens once, but interface dispatch and transport encoding/networking
still have costs. This API neither loads shared libraries at runtime nor implies
all transports can avoid copying. Routing-key extraction is independent of it.
