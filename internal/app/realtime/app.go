package realtime

import (
	"github.com/codearena-platform/codearena-core/internal/realtime"
)

type Config struct {
	Port       string // Websocket Port (e.g. :8081)
	EngineAddr string // gRPC Address of the Engine (e.g. localhost:50051)
	RedisAddr  string // Redis Address (e.g. localhost:6379)
}

func Start(cfg Config) error {
	// Original cmd/rt main.go: realtime.StartRealtime(":8081", "competition:50052")
	// We want configurable via flags.
	return realtime.StartRealtime(cfg.Port, cfg.EngineAddr, cfg.RedisAddr)
}
