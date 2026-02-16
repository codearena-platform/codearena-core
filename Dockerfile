# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for a static binary (pure-go sqlite helps here)
RUN CGO_ENABLED=0 GOOS=linux go build -o codearena ./cmd/codearena

# Final Stage
FROM alpine:latest

# Install CA certificates for secure connections
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/codearena .

# Expose ports (gRPC and Realtime/WebSocket)
EXPOSE 50051 8080

# Run the application
# Default to "start" command which orchestrates everything
ENTRYPOINT ["./codearena", "start"]
CMD ["--db-path", "/data/codearena.db"]
