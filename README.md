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
| `JWT_SECRET` | N/A | *Required* | Secret for validating WS tokens |
| `REDIS_ADDR` | N/A | `localhost:6379` | Redis address (if scaling) |

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
