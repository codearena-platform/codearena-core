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
	"github.com/codearena-platform/codearena-core/internal/runtime"
)

// Start runs the core services until a termination signal is received.
func Start(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return StartWithContext(ctx, cfg)
}

// StartWithContext runs every subsystem from the same context so a failure in
// one of them cancels the others. Shutdown order is: simulation loop, gRPC and
// HTTP servers, wait for all subsystem goroutines, then close the database.
func StartWithContext(ctx context.Context, cfg Config) error {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid engine configuration: %w", err)
	}

	fmt.Println("==================================================")
	fmt.Println("   CODEARENA CORE: UNIFIED ENGINE DAEMON   ")
	fmt.Println("==================================================")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	db, err := persistence.NewDatabase(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence: %w", err)
	}
	dbClosed := false
	defer func() {
		if !dbClosed {
			_ = db.Close()
		}
	}()

	e := services.NewSimulationEngine(cfg.ArenaWidth, cfg.ArenaHeight, db)

	if cfg.RuntimeAddr != "" {
		runtimeClient, err := runtime.NewGRPCClient(cfg.RuntimeAddr)
		if err != nil {
			return fmt.Errorf("connect to runtime: %w", err)
		}
		e.SetRuntimeClient(runtimeClient)
	}

	servers := routes.NewServers(e, tickRate(cfg.TickRate))
	servers.Simulation.SetBotRuntimeConfig(routes.BotRuntimeConfig{
		Image:            cfg.BotImage,
		ImagesByLanguage: cfg.BotImages,
		GameServerURL:    cfg.GameServer,
	})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting Simulation Service")
		if err := servers.Serve(cfg.GRPCPort, cfg.WebPort); err != nil {
			errCh <- fmt.Errorf("simulation services: %w", err)
		}
	}()

	if cfg.RunRealtime {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.Info("Starting Realtime Service (Embedded)")
			// Connects to local Engine GRPC
			if err := realtime.StartRealtimeWithContext(runCtx, cfg.RealtimePort, "localhost"+cfg.GRPCPort, cfg.RedisAddr); err != nil {
				errCh <- fmt.Errorf("embedded realtime service: %w", err)
			}
		}()
	}

	slog.Info("Core services are warming up")
	fmt.Println("[CORE] Press CTRL+C to terminate.")

	var serveErr error
	select {
	case <-runCtx.Done():
		slog.Info("Shutdown signal received")
	case err := <-errCh:
		serveErr = err
	}

	// Cancel first so background subsystems (realtime watchers, subscriptions)
	// start winding down while the servers are being drained.
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	shutdownErr := servers.Shutdown(shutdownCtx)

	wg.Wait()

	if err := db.Close(); err != nil && serveErr == nil && shutdownErr == nil {
		serveErr = fmt.Errorf("close database: %w", err)
	}
	dbClosed = true

	if shutdownErr != nil && serveErr == nil {
		serveErr = shutdownErr
	}
	return serveErr
}

func tickRate(ticksPerSecond int) time.Duration {
	if ticksPerSecond > 0 {
		return time.Second / time.Duration(ticksPerSecond)
	}
	return 16 * time.Millisecond // Default to ~60 TPS
}
