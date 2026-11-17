# WebSocket sidecar

The sidecar accepts a WebSocket connection with a `ws-user-id` header, opens an
application connection on `localhost:3001`, and registers the user. Incoming client
messages default to JSON with a string `recipientId`; see the [routing contract](../../README.md#routing-contract).
For custom wire formats, see [compiled routing plugins](../../internal/routing/README.md).
Messages for a local recipient go to that recipient's application connection.
Messages for a remote recipient are forwarded to another sidecar over HTTP.

## Build and run

Build from the **repository root**, where `go.mod` and `internal/` are available:

```sh
make build-sidecar
# Equivalent Docker build:
docker build -f cmd/sidecar/Dockerfile -t websocket-operator-sidecar:dev .
```

The current sidecar requires access to Kubernetes for discovery. In a pod it uses
its service account; outside Kubernetes it uses `KUBECONFIG` or the default
kubeconfig. Running the image by itself without cluster credentials and an
application server is not a complete demo.

The controller injects the sidecar into a labeled application Deployment. See
[the controller instructions](../controller/README.md) for the development setup.
The sidecar and application share the pod's network namespace, so `localhost`
refers to the same pod, not a different container service on a Docker bridge.

## Configuration and endpoints

| Flag | Default | Meaning |
| --- | --- | --- |
| `-port` | `3000` | WebSocket and internal HTTP listener |
| `-targetPort` | `3001` | Application WebSocket port on localhost |
| `-debug` | `false` | Enable debug logging |

Discovery currently uses namespace `default` and Service `ws-proxy-headless`.
`WS_OPERATOR_SRV_DNS_RECORD` in the example ConfigMap is a legacy setting; the
current Kubernetes discovery entry point does not read it.

`POST /message` is the internal peer-delivery endpoint: its body is one WebSocket
opcode byte followed by the payload, and its recipient is supplied by the user-ID
header. It is not a JSON-only HTTP API despite carrying JSON application messages.
An empty body for a registered recipient returns 400; an unknown recipient returns
404. A successful HTTP response is not an acknowledgment that the end user read
the message. Deploy this internal endpoint only within the intended application
network.
