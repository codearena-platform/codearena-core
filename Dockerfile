# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary. CGO_ENABLED=0 keeps the binary static for the pure-Go SQLite driver.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/codearena ./cmd/codearena

# Final Stage
FROM alpine:3.20

# ca-certificates is needed for outbound TLS (OAuth, model downloads, etc.).
# sqlite3 provides the online .backup API used by the infrastructure backup
# CronJob to take a consistent copy of /data/codearena.db while Core runs.
RUN apk --no-cache add ca-certificates sqlite \
    && mkdir -p /data \
    && chown -R 65532:65532 /data

WORKDIR /app
COPY --from=builder /out/codearena /app/codearena

# gRPC API, web/dashboard API and realtime WebSocket port.
EXPOSE 50051 50052 8081

# SQLite needs a writable /data directory, provided by the VOLUME above.
VOLUME /data

USER 65532:65532

ENTRYPOINT ["/app/codearena"]
CMD ["start", "--db-path", "/data/codearena.db"]
