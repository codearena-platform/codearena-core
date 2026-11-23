# Build Stage
# Pinned base images and package versions keep the manifest digest reproducible.
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary. CGO_ENABLED=0 keeps the binary static for the pure-Go SQLite driver.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -o /out/codearena ./cmd/codearena

# Final Stage
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc

# ca-certificates is needed for outbound TLS (OAuth, model downloads, etc.).
# sqlite3 provides the online .backup API used by the infrastructure backup
# CronJob to take a consistent copy of /data/codearena.db while Core runs.
RUN apk --no-cache add \
    ca-certificates=20261020-r0 \
    ca-certificates-bundle=20260413-r0 \
    sqlite=3.45.3-r3 \
    sqlite-libs=3.45.3-r3 \
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
