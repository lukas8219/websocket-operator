# Build stage
FROM golang:1.23.4-alpine AS build

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY ./ ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o loadbalancer ./cmd/loadbalancer/main.go
# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Add ca certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy the binary from build stage
COPY --from=build /app/loadbalancer .

# Expose the application port
EXPOSE 8080

# Command to run the executable
CMD ["./loadbalancer"] 