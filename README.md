# CodeArena Core ⚔️🛡️

**The high-performance game engine for AI combat**

[![Go Report Card](https://goreportcard.com/badge/github.com/codearena-platform/codearena-core)](https://goreportcard.com/report/github.com/codearena-platform/codearena-core)
[![Release](https://img.shields.io/github/v/release/codearena-platform/codearena-core)](https://github.com/codearena-platform/codearena-core/releases)
[![Docker](https://img.shields.io/docker/v/codearena/core)](https://hub.docker.com/r/codearena/core)

CodeArena Core runs deterministic bot battles. It owns the simulation tick loop,
the sandboxed bot runtime, replay persistence and real-time state streaming.

## Features

- **Deterministic physics engine** with a fixed 60 TPS default.
- **Sandboxed bots** in Docker containers with strict CPU, memory, PID and
  filesystem restrictions.
- **Real-time streaming** through a dashboard WebSocket on the engine web port
  and an optional standalone realtime WebSocket service backed by Redis Pub/Sub.
- **SQLite-only persistence** via GORM for matches, bots, event logs and
  highlights.
- **Hybrid deployment**: one process for local development (`codearena start`)
  or separate engine/realtime/runtime services.

## Install

### Docker

```bash
docker run --rm \
  -p 50051:50051 \
  -p 50052:50052 \
  -p 8081:8081 \
  -v "$(pwd)/data:/data" \
  -e RUNTIME_AUTH_TOKEN='replace-me' \
  codearena/core:latest
```

The image runs `codearena start --db-path /data/codearena.db` as a non-root
user. The container always needs `RUNTIME_AUTH_TOKEN` because the local runtime
gRPC API is authenticated; for local development only, set
`RUNTIME_ALLOW_INSECURE=true` instead.

### Native binaries

The release workflow publishes archives for Linux, Windows and macOS (amd64 and
arm64), plus a Linux `.deb`, a Windows MSI and a Homebrew tap. Download the
artifact for your platform from the
[Releases page](https://github.com/codearena-platform/codearena-core/releases).

## CLI

The unified `codearena` binary has four commands:

```bash
# Engine + embedded realtime + local runtime (development)
codearena start

# Engine only; use --no-realtime when realtime runs separately
codearena engine --runtime-addr=10.0.0.5:50053 --no-realtime

# Standalone services
codearena runtime --port 50053 --max-concurrent-bots 10
codearena realtime --realtime-port :8081 --engine-addr localhost:50051

# Replay inspection
codearena replay list
codearena replay highlights match-id-123
codearena replay logs match-id-123 --start 100 --end 200
```

Individual standalone binaries (`cmd/engine`, `cmd/runtime`, `cmd/realtime`)
remain available and read the legacy environment variables listed below.

## Ports

| Port | Service | Protocol |
|---|---|---|
| `50051` | Engine Match/Simulation/Bot API | gRPC |
| `50052` | Engine dashboard data (`/ws`) | HTTP/WebSocket |
| `50053` | Runtime bot management | gRPC (authenticated) |
| `8081` | Standalone/embedded realtime client socket | WebSocket |

The old `8080` port and `--port` flag on the engine do not exist. `--port` is
only used by `codearena runtime` to select its gRPC listen port.

## Configuration

CLI flags take precedence over environment variables. The CLI reads `CODEARENA_`
prefixed variables with `-` replaced by `_` (for example `--grpc-port` maps to
`CODEARENA_GRPC_PORT`). The standalone binaries accept the legacy aliases in
parentheses.

| Flag | Environment variable | Default | Description |
|---|---|---|---|
| `--grpc-port` | `CODEARENA_GRPC_PORT` (`GRPC_PORT`) | `:50051` | Engine gRPC listener |
| `--web-port` | `CODEARENA_WEB_PORT` (`WEB_PORT`) | `:50052` | Engine dashboard listener |
| `--realtime-port` | `CODEARENA_REALTIME_PORT` (`REALTIME_PORT`) | `:8081` | Realtime WebSocket listener |
| `--runtime-addr` | `CODEARENA_RUNTIME_ADDR` (`RUNTIME_ADDR`) | `localhost:50053` | Runtime gRPC target; empty on `start` disables the client |
| `--no-realtime` | `CODEARENA_NO_REALTIME` (`NO_REALTIME`) | `false` | Do not embed realtime in the engine |
| `--arena-width` | `CODEARENA_ARENA_WIDTH` | `800` | Arena width |
| `--arena-height` | `CODEARENA_ARENA_HEIGHT` | `600` | Arena height |
| `--tick-rate` | `CODEARENA_TICK_RATE` | `60` | Simulation ticks per second (1–240) |
| `--redis-addr` | `CODEARENA_REDIS_ADDR` (`REDIS_ADDR`) | `localhost:6379` | Redis for realtime fan-out |
| `--db-path` | `CODEARENA_DB_PATH` (`DB_PATH`) | `codearena.db` | SQLite database file |
| `--engine-addr` | `CODEARENA_ENGINE_ADDR` (`ENGINE_ADDR`) | `localhost:50051` | Engine gRPC target for `codearena realtime` |
| `--max-concurrent-bots` | `CODEARENA_MAX_CONCURRENT_BOTS` | `10` | Runtime container capacity |
| N/A | `JWT_SECRET` | *required* | HS256 secret for dashboard WebSocket tokens |
| N/A | `JWT_ISSUER` | `codearena` | Expected JWT issuer |
| N/A | `JWT_AUDIENCE` | `codearena-clients` | Expected JWT audience |
| N/A | `WS_ALLOWED_ORIGINS` | *same-origin* | Comma-separated WebSocket origin allowlist |
| N/A | `LOG_LEVEL` / `LOG_FORMAT` | `INFO` / text | CLI logging |
| N/A | `RUNTIME_AUTH_TOKEN` | *required* | Shared secret for runtime gRPC calls |
| N/A | `RUNTIME_ALLOW_INSECURE` | `false` | Local-dev opt-out from runtime auth |
| N/A | `CODEARENA_BOT_IMAGE` | *unset* | Container image for `RegisterBot` managed bots |
| N/A | `CODEARENA_BOT_IMAGE_<LANGUAGE>` | *unset* | Per-language image override, e.g. `CODEARENA_BOT_IMAGE_PYTHON` |
| N/A | `CODEARENA_GAME_SERVER_URL` | `localhost:50051` | `GAME_SERVER_URL` passed to bot containers |

### Persistence

Persistence is SQLite only. There is no PostgreSQL driver or configuration in
this repository. Run the engine with a writable `--db-path` (for example
`/data/codearena.db` in Docker).

### Runtime and WebSocket security

The Runtime gRPC API requires `RUNTIME_AUTH_TOKEN`. Clients send it as
`Authorization: Bearer <token>` (or `x-runtime-token`). gRPC reflection is only
registered with the explicit local-development opt-out. In production
(`APP_ENV=production` or `CODEARENA_ENV=production`), the opt-out is refused.

Every bot container runs as a non-root user with all capabilities dropped,
no-new-privileges, Docker's default seccomp profile, a read-only root filesystem
(plus a small `tmpfs` at `/tmp`), 512 MiB memory (no swap), 0.5 CPU, 64 PIDs, a
start timeout and a 5-minute execution deadline. Containers are removed on stop,
on failed start and when the deadline expires. Optional overrides:
`RUNTIME_BOT_MEMORY_BYTES`, `RUNTIME_BOT_MEMORY_SWAP_BYTES`,
`RUNTIME_BOT_NANO_CPUS`, `RUNTIME_BOT_PIDS_LIMIT`, `RUNTIME_BOT_NETWORK`
(default `none`), `RUNTIME_BOT_TIMEOUT`, `RUNTIME_BOT_START_TIMEOUT`,
`RUNTIME_BOT_STOP_TIMEOUT`, `RUNTIME_BOT_USER`, `RUNTIME_READONLY_ROOTFS`,
`RUNTIME_SECCOMP_PROFILE`, `RUNTIME_ALLOWED_IMAGES` (comma-separated exact
references) and `RUNTIME_REQUIRE_DIGEST`. An image allowlist requires
digest-pinned references by default, for example `codearena/bot-go@sha256:...`.

With the default `RUNTIME_BOT_NETWORK=none`, bots cannot reach the game server
over the network. Set an explicit Docker network only for trusted local
development.

Dashboard WebSocket clients must send `Authorization: Bearer <JWT>`; query
tokens are rejected. Tokens must be HS256 and include `exp`, `iss` and `aud`.
Browser origins must match `WS_ALLOWED_ORIGINS` or the request host.

### Match and bot lifecycle

`MatchService.CreateMatch` validates the arena, persists a `WAITING` match when
a database is configured, and resets the in-process simulation. It refuses to
reset a running match. `StartSimulation` marks the match `RUNNING` and
`StopSimulation` marks it `FINISHED` and stops managed bot containers.

`MatchService.RegisterBot` requires `user_id` and `name` and an existing match.
It returns generated `bot_id`/`version_id` values. `source_code` is accepted
but is not persisted by Core v1. When a bot image is configured, the engine
calls Runtime `StartBot`; otherwise bots connect directly through
`BotService.Connect`.

### Graceful shutdown

`SIGINT`/`SIGTERM` (or a subsystem failure) cancels one context shared by the
engine, embedded realtime and local runtime. Shutdown stops the game loop and
stream channels, drains gRPC with `GracefulStop` (forced `Stop` after 5s),
shuts down HTTP, waits for subsystem goroutines, and only then closes SQLite.
The runtime stops every bot container it created.

## API documents

- Protobuf contracts: `api/proto/v1/*.proto`
- HTTP/WebSocket surface and gRPC method index: `docs/api/openapi.yaml`
- Regenerate: `./scripts/generate_api.sh` (or `scripts/generate_api.ps1` on Windows)

## Architecture

1. **Simulation Engine** — physics, tick loop and match state.
2. **Runtime Service** — Docker bot sandbox and scheduler.
3. **Realtime Service** — WebSocket fan-out via Redis.
4. **Match Service** — gRPC match, replay and highlight APIs.

## Development

```bash
go build -o codearena ./cmd/codearena
go test ./...
go test -race ./internal/engine/... ./internal/runtime/... ./internal/app/...
```

## License

MIT License © 2026 CodeArena Platform
