FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy dependencies first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY ./ ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o webhook-server ./cmd/controller/

# Create a minimal runtime image
FROM alpine:3.19

WORKDIR /app

# Install CA certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Create directory for TLS certificates
RUN mkdir -p /etc/webhook/certs

# Copy the binary from the builder stage
COPY --from=builder /app/webhook-server .

# Expose the default port
EXPOSE 8080

# Run the webhook server
ENTRYPOINT ["/app/webhook-server"] 