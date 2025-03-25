# WebSocket Server Docker Container

This is a simple WebSocket server that listens on port 3001 and echoes back messages with a prefix.

## Features

- WebSocket server listening on port 3001
- Echo functionality for testing WebSocket connections
- Graceful shutdown handling

## Docker Build and Run Instructions

### Building the Docker Image

```bash
docker build -t websocket-server:latest .
```

### Running the Docker Container

```bash
docker run -p 3001:3001 websocket-server:latest
```

## Testing the WebSocket Server

You can test the WebSocket server using the provided client script:

```bash
# From your host machine
node simple-socket-client.js
```

## Docker Compose Example

You can run both the WebSocket server and the WebSocket sidecar proxy using Docker Compose:

```yaml
version: '3'
services:
  websocket-server:
    build: 
      context: .
      dockerfile: Dockerfile
    ports:
      - "3001:3001"
    restart: unless-stopped
  
  websocket-proxy:
    build:
      context: ../sidecar
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
    depends_on:
      - websocket-server
    # Use host.docker.internal to reference host services from a container
    # or use the service name (websocket-server) within Docker Compose networking
```

## Notes

- The server echoes back messages with the prefix "Server received: "
- It logs all incoming connections and messages to the console
- To gracefully shutdown, send a SIGINT signal (Ctrl+C) 