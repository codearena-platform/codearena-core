# CodeArena Core ⚔️🛡️

**The High-Performance Game Engine for AI Combat**

[![Go Report Card](https://goreportcard.com/badge/github.com/codearena-platform/codearena-core)](https://goreportcard.com/report/github.com/codearena-platform/codearena-core)
[![Release](https://img.shields.io/github/v/release/codearena-platform/codearena-core)](https://github.com/codearena-platform/codearena-core/releases)
[![Docker](https://img.shields.io/docker/v/codearena/core)](https://hub.docker.com/r/codearena/core)

CodeArena Core is the mission-critical backend that powers industrial-grade bot battles. It handles physics simulation, real-time broadcasting, bot isolation, and match persistence with microsecond precision.

## 🚀 Key Features

*   **Deterministic Physics Engine**: Custom 2D rigid-body physics tailored for reproducible combat scenarios.
*   **Secure Runtime**: Sandboxed execution of user-submitted bots using Docker containers with strict resource limits and network isolation.
*   **Real-time Broadcasting**: Low-latency state synchronization via WebSockets (JWT Authenticated) + Redis PubSub for horizontal scaling.
*   **Persistence & Replays**: 
    *   **Storage**: SQLite (default) or PostgreSQL support via GORM.
    *   **Replay System**: Records every tick of the simulation.
    *   **Highlights**: Automatically detects key moments (kills, wins) for instant playback.
*   **Hybrid Delivery**: Available as a lightweight Docker container (~15MB), native host binaries, or system installers (MSI/DEB).

## 📦 Installation

### Docker (Recommended)
```bash
docker pull codearena/core:latest
docker run -p 50051:50051 -p 8080:8080 -v $(pwd)/data:/data codearena/core:latest
```

### Native Binaries
Download the latest release for your OS from the [Releases Page](https://github.com/codearena-platform/codearena-core/releases).
*   **Windows**: Download `.msi` updater or portable `.zip`.
*   **Linux (Ubuntu/Debian)**: `sudo dpkg -i codearena_linux_amd64.deb`
*   **macOS**: `brew install codearena-platform/tap/codearena`

## 🕹️ CLI Usage

The core binary `codearena` is your main entry point.

### Start the Server
Launch the full stack (Engine + Realtime + Runtime):
```bash
codearena start --port 8080 --grpc-port 50051 --db-path ./arena.db
```

### Replay Tools
Explore past matches directly from the terminal:
```bash
# List all recorded matches
codearena replay list

# View highlights for a specific match
codearena replay highlights match-id-123

# Dump raw event logs for debugging
codearena replay logs match-id-123 --start 100 --end 200
```

## ⚙️ Configuration

CodeArena Core can be configured via flags or environment variables (12-factor app compliant).

| Env Variable | Flag | Default | Description |
|---|---|---|---|
| `CODEARENA_PORT` | `--port` | `8080` | HTTP/WebSocket port |
| `CODEARENA_GRPC_PORT` | `--grpc-port` | `50051` | gRPC Service port |
| `CODEARENA_DB_PATH` | `--db-path` | `codearena.db` | Path to SQLite DB file |
| `JWT_SECRET` | N/A | *Required* | Secret for validating WebSocket access tokens (HS256) |
| `JWT_ISSUER` | N/A | `codearena` | Expected JWT issuer |
| `JWT_AUDIENCE` | N/A | `codearena-clients` | Expected JWT audience |
| `WS_ALLOWED_ORIGINS` | N/A | *same-origin* | Comma-separated WebSocket origin allowlist |
| `RUNTIME_AUTH_TOKEN` | N/A | *Required* | Shared secret for all Runtime gRPC calls |
| `RUNTIME_ALLOW_INSECURE` | N/A | `false` | Local-dev only opt-out from the token requirement |

### Runtime and WebSocket security

The Runtime gRPC API requires `RUNTIME_AUTH_TOKEN`. Clients send it as
`Authorization: Bearer <token>` (or `x-runtime-token`). gRPC reflection is only
registered when the explicit local-development opt-out is active. In production
(`APP_ENV=production` or `CODEARENA_ENV=production`) the opt-out is refused.

For local development only, both the token and reflection can be disabled with:

```bash
export RUNTIME_ALLOW_INSECURE=true
```

Every bot container runs as a non-root user with all capabilities dropped,
no-new-privileges, Docker's default seccomp profile, a read-only root filesystem
(plus a small `tmpfs` at `/tmp`), 512 MiB memory (no swap), 0.5 CPU, 64 PIDs, a
start timeout and a 5-minute execution deadline. Containers are removed on stop,
on failed start and when the deadline expires. Optional overrides:
`RUNTIME_BOT_MEMORY_BYTES`, `RUNTIME_BOT_NANO_CPUS`, `RUNTIME_BOT_PIDS_LIMIT`,
`RUNTIME_BOT_NETWORK` (default `none`), `RUNTIME_BOT_TIMEOUT`,
`RUNTIME_BOT_START_TIMEOUT`, `RUNTIME_BOT_STOP_TIMEOUT`, `RUNTIME_BOT_USER`,
`RUNTIME_READONLY_ROOTFS`, `RUNTIME_SECCOMP_PROFILE`, `RUNTIME_ALLOWED_IMAGES`
(comma-separated exact references) and `RUNTIME_REQUIRE_DIGEST`. Setting an
image allowlist requires digest-pinned references (for example
`codearena/bot-go@sha256:...`) by default.

Note: with the default `RUNTIME_BOT_NETWORK=none`, bots cannot reach the game
server over the network. Set an explicit Docker network only for trusted
local development.

WebSocket clients must send `Authorization: Bearer <JWT>`; query-string tokens
are rejected. Tokens must be HS256 and include `exp`, `iss` and `aud`. Browser
origins must match `WS_ALLOWED_ORIGINS` or the request host.

## 🏗️ Architecture

The Core is a unified Go module composed of decoupled services:

1.  **Simulation Engine**: The "Brain". Runs the game loop at fixed ticks (e.g., 60Hz).
2.  **Runtime Service**: The "Jailer". Manages Docker containers for bots.
3.  **Realtime Service**: The "Broadcaster". Pushes WorldState via WebSockets.
4.  **Match Service**: The "Librarian". Stores and retrieves replays via gRPC.

## 🛠️ Development

### Prerequisites
*   Go 1.22+
*   Docker (for running bots)
*   Protoc (for regenerating gRPC stubs)

### Build
```bash
go build -o codearena ./cmd/codearena
```

### Test
Run the test suite (includes 80/20 Pareto coverage for critical paths):
```bash
go test ./...
```

## 📄 License
MIT License © 2026 CodeArena Platform
