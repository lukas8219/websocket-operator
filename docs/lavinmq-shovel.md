# LavinMQ HTTP shovel compatibility

Issue #65 proposes either native AMQP delivery or HTTP shovels. This probe runs
against a dedicated LavinMQ 2.11.1 instance and checks its actual wire mapping:

```sh
LAVINMQ_HTTP_URL=http://127.0.0.1:15672 \
LAVINMQ_AMQP_URL=amqp://guest:guest@127.0.0.1:5672/ \
python3 scripts/test-lavinmq-shovel.py
```

The script owns a random temporary queue/shovel and removes both on completion.
It starts a local HTTP capture server; run the broker on the same host so its
loopback destination reaches that server. CI uses the checksummed native binary.

The body arrives unchanged, including the leading WebSocket opcode. However,
AMQP header `ws-user-id` arrives as HTTP `X-ws-user-id`, so it cannot satisfy the
default sidecar header contract. A deployment using `WS_OPERATOR_USER_HEADER=X-App-User`
can instead publish AMQP header `App-User`. The probe verifies both mappings; it
captures HTTP deliveries rather than claiming to exercise the whole operator.

Shovels also change delivery behavior: their HTTP destination waits for a response,
retries transient failures, and can redeliver messages. An HTTP success still does
not prove an end user consumed the message. Per-pod queue/shovel lifecycle,
recipient movement, 404 handling, duplicates, backpressure and management API
credentials need an explicit production design before the operator provisions them.
This PR does not add broker resources or change the production transport.

Sources: [LavinMQ 2.11.1 shovel semantics](https://github.com/cloudamqp/lavinmq/blob/v2.11.1/docs/shovels.md)
and [HTTP destination implementation](https://github.com/cloudamqp/lavinmq/blob/v2.11.1/src/lavinmq/shovel/http_destination.cr).
