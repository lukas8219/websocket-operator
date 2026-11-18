# LavinMQ transport contract

LavinMQ is a compiled Go transport backend for #65, using AMQP 0-9-1 directly.
HTTP remains the default. Select `WS_OPERATOR_TRANSPORT_PLUGIN=lavinmq` to
use this backend through the same `Transport.Write` interface.

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

## Enable the plugin

Use an image containing this change for both the controller and sidecar. Install
LavinMQ separately; the operator does not provision it. All replicas must use the
same transport, broker/vhost, discovery namespace, and discovery service.

For an already injected Deployment named `chat`, create a Kubernetes Secret named
`ws-lavinmq` with key `WS_OPERATOR_LAVINMQ_URL` containing its AMQP URL. Supply the
value through your secret manager or a protected file; do not store it in the
shared ConfigMap.

The updated injector supplies `WS_OPERATOR_POD_IP` from `status.podIP`. Existing
injected containers need that downward-API environment variable added explicitly:

```yaml
# Merge into spec.template.spec.containers[name=websocket-proxy].env
- name: WS_OPERATOR_POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
```

Once the Secret and pod-IP variable are in place, enable the injected container:

```sh
kubectl set env deployment/chat --containers=websocket-proxy \
  --from=secret/ws-lavinmq WS_OPERATOR_TRANSPORT_PLUGIN=lavinmq
kubectl rollout status deployment/chat
```

Manual sidecars need the same variable. Its IP and the sidecar listen port must
match the discovery endpoint (normally port 3000). The queue prefix is
`ws-operator.<namespace>.<service>`, using the existing discovery configuration.
Coordinate the restart of all replicas: mixed HTTP/LavinMQ pods can lose messages.
Broker loss or a publish timeout exits the sidecar; Deployments restart it under
`restartPolicy: Always`. Restart does not recover the temporary queue or retry
uncertain messages. SIGTERM closes the broker connection as the server stops.

Run broker-backed tests with `LAVINMQ_AMQP_URL=amqp://guest:guest@127.0.0.1:5672/`
and `go test -race ./internal/transports/... ./cmd/sidecar`. CI starts a
checksum-pinned LavinMQ 2.11.1 process and also exercises network failures.
