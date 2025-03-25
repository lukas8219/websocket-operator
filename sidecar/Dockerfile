# Build stage
FROM golang:1.22.4-alpine AS build

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY main.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sidecar .

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Add ca certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy the binary from build stage
COPY --from=build /app/sidecar .

# Expose the application port
EXPOSE 3000

# Command to run the executable
CMD ["./sidecar"] 