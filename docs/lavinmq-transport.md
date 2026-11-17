# LavinMQ transport contract

LavinMQ is a compiled Go transport backend for #65, using AMQP 0-9-1 directly.
HTTP remains the default. This package implements `Transport.Write`; sidecar
registration and deployment configuration are added separately.

Each sidecar owns one exclusive, non-durable queue named from a service prefix
and the SHA-256 of its discovery socket address. All sidecars in the same service
must share a broker/vhost and prefix. Different services must use different
prefixes. A lookup still chooses the recipient's pod; AMQP carries its routing
key, WebSocket opcode, and unchanged payload to that pod's local delivery callback.
This does not change load-balancer routing or manage a broker/shovel for you.

`Write` waits for a publisher confirmation. Mandatory publishing detects missing
target queues. Success means the broker accepted the message, not that the user
received it. Application failures and malformed envelopes are rejected without
requeueing. There is no persistence, offline inbox, automatic retry, or exactly-once
guarantee. A recipient moving between pods can lose an in-flight message.

Queues expire messages after five seconds and hold at most 256 messages / 16 MiB.
Each message is at most 16 MiB. Queue overflow rejects the publish. Only text and
binary WebSocket messages are supported. Each consumer has one unacknowledged
message at a time; each publisher has one outstanding confirmation. The default
five-second timeout bounds admission and publishing. Inputs are borrowed only
until `Write` returns; publishing copies them before the AMQP client uses them.

Broker/channel loss or an ambiguous publish timeout closes `Done`. The host must
stop serving and restart the backend. It must not silently retry the failed
message, which the broker might already have accepted. `Close` is idempotent;
a local delivery already running may finish. Callbacks must bound their I/O.
Use a dedicated least-privilege broker user/vhost and `amqps://` outside trusted
networks; connection errors do not echo the credential-bearing URL.
