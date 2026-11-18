# LavinMQ transport contract

LavinMQ is a compiled Go plugin shared by the load balancer and sidecar.
`internal/transports/builtin.Factories` is their single registration point.
The default `http` plugin uses WebSocket sessions and HTTP cross-sidecar delivery.
Selecting `lavinmq` replaces the entire internal link with AMQP: session open,
application readiness, messages, replies, close, and liveness. External clients
and application servers still use WebSockets. See [the design](design/shared-transport.md).

Each load balancer and sidecar owns an exclusive, temporary inbox named from the
service prefix and a hash of its pod IP/listen port. Random session IDs return
replies to the owning load balancer. The sidecar still resolves recipients and
routes cross-sidecar messages. A ready reply means the application socket opened.

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

## Enable the plugin

Build all three images from the same revision. Install LavinMQ separately and
create a Secret named `ws-lavinmq`, with key `WS_OPERATOR_LAVINMQ_URL`, in the
application/release namespace. Supply its URL through your secret manager or a
protected file. All load balancers and sidecars must share the same broker/vhost,
plugin, discovery namespace, and discovery service.

For Helm, use:

```sh
helm upgrade --install chat charts/websocket-operator -n chat \
  --set transport.name=lavinmq --set transport.existingSecret=ws-lavinmq
```

The chart configures the load balancer and tells the controller to inject the
same Secret reference. The controller does not read the Secret's contents.
For plain manifests, set `WS_OPERATOR_TRANSPORT_PLUGIN=lavinmq` in the shared
ConfigMap, add `--transport-secret=ws-lavinmq` to the controller arguments, and
reference the Secret key in the load-balancer container's environment:

```yaml
- name: WS_OPERATOR_LAVINMQ_URL
  valueFrom:
    secretKeyRef:
      name: ws-lavinmq
      key: WS_OPERATOR_LAVINMQ_URL
```

Existing injected sidecars also need that Secret reference; the injector skips
containers already named `websocket-proxy`. All endpoints need pod identity:

```yaml
- name: WS_OPERATOR_POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
```

The chart, load-balancer manifest, and injector supply pod identity. Manual
sidecars must match the discovery address (normally pod IP, port 3000).
The shared-link queue prefix is `ws-operator.link.<namespace>.<service>`.
It deliberately differs from the old one-way plugin's queue schema.
Coordinate deployment of **both roles** and reconnect clients: mixed versions or
mixed plugins cannot share sessions. Updating a ConfigMap or Secret alone does
not refresh process environment; restart existing pods after configuration changes.

Broker loss or an ambiguous publish timeout closes every local session and the
HTTP listeners. Deployments restart the process; there is no replay. Idle sessions
send heartbeats every five seconds and expire after twenty seconds without peer
activity. A missing destination queue is detected on the next publish.
Each endpoint admits at most 256 sessions; session inboxes hold sixteen messages
with a shared 16 MiB budget. User IDs are limited to 512 bytes. The versioned wire
header reserves 4101 bytes from the 16 MiB message limit; payload bytes are raw,
not base64. Use separate broker permissions for mutually untrusted applications;
source addresses in messages are routing checks, not authentication.

Run `LAVINMQ_AMQP_URL=amqp://guest:guest@127.0.0.1:5672/ go test -race
./internal/transports/... ./cmd/sidecar ./cmd/loadbalancer/...` against a local
broker. CI uses checksum-pinned LavinMQ 2.11.1.
