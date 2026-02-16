# CodeArena Core: The Engine Stack 🏛️

**Organization**: [github.com/codearena-platform](https://github.com/codearena-platform)

High-performance, mission-critical infrastructure for the CodeArena ecosystem. This repository contains the physics engine, real-time broadcasting, and execution runtime that power industrial-grade bot combat.

## Key Modules
The Core is a unified Go module `github.com/codearena-platform/core`:
- **`pkg/types`**: The "Source of Truth" for the entire network layer.
- **`internal/simulation`**: The Match-Making and Physics engine (Deterministic Combat).
- **`internal/realtime`**: Low-latency WebSocket broadcasting for battle state.
- **`internal/runtime`**: Bot sandboxing and isolation layer.

## Concurrency & Scaling
CodeArena Core is designed for extreme horizontal and vertical scaling. 

While influenced by the performance patterns of Rust's **Tokio**, CodeArena Core leverages the native Go **Goroutine Scheduler** to manage thousands of concurrent battle processes across multiple threads with microscopic memory footprint.
- **M:N Scheduling**: Millions of logical tasks mapped to a small pool of OS threads.
- **Work-Stealing**: Dynamic task distribution across CPU cores to prevent bottlenecks.
- **Non-Blocking I/O**: High-concurrency WebSocket and gRPC handled via efficient epoll/kqueue abstractions.

## Modular Integration
This repository supports two primary modes of operation:
1. **Standalone Microservices**: Each module can be built as a separate binary via `cmd/`.
2. **Unified Core Daemon**: Use `cmd/core/main.go` to launch all core services within a single high-performance process, managed by internal Goroutine workers.

### Building
```bash
# Build the integrated core
go build -o codearena-core ./cmd/core/main.go

# Build specific standalone services
go build -o sim ./cmd/sim
go build -o rt ./cmd/rt
```

## Standalone Usage
This repository is fully decoupled. To use core types in other repositories:
```bash
go get github.com/codearena-platform/core/pkg/types
```

---
*CodeArena Core is the primary engine stack of the CodeArena-Platform.*
