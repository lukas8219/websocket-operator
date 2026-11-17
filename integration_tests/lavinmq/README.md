# LavinMQ transport experiment

Run a dedicated LavinMQ 2.11.1 broker, then:

```sh
LAVINMQ_AMQP_URL=amqp://guest:guest@127.0.0.1:5672/ go test -tags integration \
  -run '^$' -bench BenchmarkReceiverAcceptance -benchmem -benchtime=300ms \
  -count=3 ./integration_tests/lavinmq
```

The experiment compares one outstanding message over reused HTTP connections
with AMQP direct-queue delivery plus an explicit receiver reply. Both receivers
validate the recipient and complete opcode-prefixed payload before acceptance.
A publisher confirm alone would measure broker acceptance and is not equivalent.
Queues are exclusive, auto-deleted, non-durable; AMQP uses separate publisher and
receiver connections, prefetch one, and explicit receiver acknowledgments.

This is a local serial acceptance baseline, not a production transport or a
saturation benchmark. It excludes WebSocket delivery, discovery, reconnects,
persistence, TLS and broker failure recovery. Go allocation results include the
benchmark's publisher and receiver, but exclude the external broker. Capture CPU,
Go/broker versions and raw samples; CI does so without timing thresholds.

Before adopting AMQP, define per-pod queue routing, stale recipient handling,
backpressure, redelivery/deduplication, and what successful Write promises. A
broker introduces additional ownership and operational costs; using TCP does not
by itself establish an advantage over HTTP, which also runs over TCP.
