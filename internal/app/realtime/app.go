package realtime

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/codearena-platform/codearena-core/internal/realtime"
)

type Config struct {
	Port       string // Websocket Port (e.g. :8081)
	EngineAddr string // gRPC Address of the Engine (e.g. localhost:50051)
	RedisAddr  string // Redis Address (e.g. localhost:6379)
}

func Start(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return StartWithContext(ctx, cfg)
}

// startRealtime is a variable so the CLI can be tested without opening sockets.
var startRealtime = realtime.StartRealtimeWithContext

func StartWithContext(ctx context.Context, cfg Config) error {
	// Original cmd/rt main.go: realtime.StartRealtime(":8081", "competition:50052")
	// We want configurable via flags.
	return startRealtime(ctx, cfg.Port, cfg.EngineAddr, cfg.RedisAddr)
}
