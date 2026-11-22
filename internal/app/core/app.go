package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	DBPath      string // Path to SQLite database file
}

func Start(cfg Config) error {
	fmt.Println("==================================================")
	fmt.Println("   CODEARENA CORE: UNIFIED ENGINE DAEMON   ")
	fmt.Println("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	servers := routes.NewServers(e, tickRate(cfg.TickRate))
	errCh := make(chan error, 2)
	go func() {
		slog.Info("Starting Simulation Service")
		if err := servers.Serve(cfg.GRPCPort, cfg.WebPort); err != nil {
			errCh <- fmt.Errorf("simulation services: %w", err)
		}
	}()

	// 3. Start Realtime Service (Optional)
	if cfg.RunRealtime {
		go func() {
			slog.Info("Starting Realtime Service (Embedded)")
			// Connects to local Engine GRPC
			if err := realtime.StartRealtimeWithContext(ctx, cfg.RealtimePort, "localhost"+cfg.GRPCPort, cfg.RedisAddr); err != nil {
				errCh <- fmt.Errorf("embedded realtime service: %w", err)
			}
		}()
	}

	slog.Info("Core services are warming up")
	fmt.Println("[CORE] Press CTRL+C to terminate.")

	// Wait for termination signal or a server failure.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	var serveErr error
	select {
	case <-stop:
		slog.Info("Shutdown signal received")
	case err := <-errCh:
		serveErr = err
	case <-ctx.Done():
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := servers.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return serveErr
}

func tickRate(ticksPerSecond int) time.Duration {
	if ticksPerSecond > 0 {
		return time.Second / time.Duration(ticksPerSecond)
	}
	return 16 * time.Millisecond // Default to ~60 TPS
}
