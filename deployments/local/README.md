# Demo WebSocket application and client

This directory contains a Node.js example application, an interactive client, and
a legacy CoreDNS-only Compose file. It does not run the complete operator through
Docker Compose.

## Run the application directly

From this directory, with Node.js installed:

```sh
npm ci
node simple-socket-server.js 3001
```

The server sends a welcome message on connection and echoes received data as
`Server received: ...`. It is a demo application, not a WebSocket conformance
fixture: it converts incoming data to a string and adds a prefix.

To build its image from the repository root:

```sh
docker build -f deployments/local/Dockerfile -t websocket-operator-test-server:dev deployments/local
```

## Use the client through the operator

First deploy the operator and demo application using the
[controller instructions](../../cmd/controller/README.md), then expose the load
balancer from the repository root:

```sh
kubectl port-forward service/websocket-operator-loadbalancer 8080:8080
```

In this directory, open two terminals and run:

```sh
node simple-socket-client.js --user alice --recipientId bob --port 8080
node simple-socket-client.js --user bob --recipientId alice --port 8080
```

Type messages in either terminal to address the other user.
The client supplies the user-ID header and sends a JSON `recipientId` string.

Messages return through the recipient's application connection. Connecting the
client directly to the Node.js server tests that application alone; it does not
exercise user sharding, peer discovery, or sidecar routing.

For automated component tests, use `make test` and `make test-race` at the repository
root. [Kubernetes discovery tests](../../integration_tests/README.md) require a
separate test cluster. The old CoreDNS Compose file is not needed by the current
Kubernetes discovery implementation.
