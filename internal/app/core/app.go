package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/routes"
	"github.com/codearena-platform/codearena-core/internal/engine/services"
	"github.com/codearena-platform/codearena-core/internal/persistence"
	"github.com/codearena-platform/codearena-core/internal/realtime"
)

type Config struct {
	GRPCPort string
	WebPort  string

	// Realtime Configuration
	RunRealtime  bool   // If true, runs Realtime service within this process
	RealtimePort string // Only used if RunRealtime is true

	RuntimeAddr string // Address of the Runtime Service (e.g. localhost:50052)

	ArenaWidth  float32
	ArenaHeight float32
	TickRate    int    // Ticks per second
	RedisAddr   string // Redis address for horizontal scaling
	DBPath      string // Path to SQLite database
}

func Start(cfg Config) error {
	fmt.Println("==================================================")
	fmt.Println("   CODEARENA CORE: UNIFIED ENGINE DAEMON   ")
	fmt.Println("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// 0. Initialize Persistence
	db, err := persistence.NewDatabase(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence: %w", err)
	}
	defer db.Close()

	// 1. Initialize Simulation Engine
	// TODO: Pass RuntimeAddr to Engine so it can find the Runtime Service
	e := services.NewSimulationEngine(cfg.ArenaWidth, cfg.ArenaHeight, db)

	// 2. Start Simulation gRPC & Web
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting Simulation Service")
		tickRate := 16 * time.Millisecond // Default to ~60 TPS (1000ms / 60 = 16.66ms)
		if cfg.TickRate > 0 {
			tickRate = time.Second / time.Duration(cfg.TickRate)
		}
		routes.StartAll(cfg.GRPCPort, cfg.WebPort, e, tickRate)
	}()

	// 3. Start Realtime Service (Optional)
	if cfg.RunRealtime {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.Info("Starting Realtime Service (Embedded)")
			// Connects to local Engine GRPC
			realtime.StartRealtime(cfg.RealtimePort, "localhost"+cfg.GRPCPort, cfg.RedisAddr)
		}()
	}

	slog.Info("Core services are warming up")
	fmt.Println("[CORE] Press CTRL+C to terminate.")

	// Wait for termination signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		slog.Info("Shutdown signal received")
	case <-ctx.Done():
	}

	return nil
}
