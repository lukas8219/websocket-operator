# WebSocket Proxy Sidecar

This is a WebSocket proxy sidecar container that forwards connections from port 3000 to a target WebSocket server on localhost:3001.

## Functionality

The sidecar acts as a proxy for WebSocket connections:
- Listens for incoming WebSocket connections on port 3000
- Forwards these connections to a WebSocket server on port 3001
- Bidirectionally proxies messages between clients and the target WebSocket server

## Docker Build and Deployment

### Building the Docker Image

```bash
docker build -t socketio-sidecar:latest .
```

### Running the Docker Container

```bash
# Basic usage
docker run -p 3000:3000 socketio-sidecar:latest

# If you need to connect to a service on the host
docker run -p 3000:3000 --network="host" socketio-sidecar:latest
```

### Using in Kubernetes

This container is designed to run as a sidecar in a Kubernetes Pod alongside your main application. Example configuration in a Pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app-with-sidecar
spec:
  containers:
  - name: main-app
    image: your-main-app:latest
    ports:
    - containerPort: 3001
  - name: websocket-proxy
    image: socketio-sidecar:latest
    ports:
    - containerPort: 3000
```

## Environment Variables

Currently, the application doesn't support configuration through environment variables. It uses hardcoded ports:
- Port 3000 for incoming connections
- Port 3001 for the target WebSocket server

## Notes

- The proxy expects the target WebSocket server to be accessible at `ws://localhost:3001`
- Ensure proper network configuration if running in Docker or Kubernetes to allow the sidecar to reach the target WebSocket server 