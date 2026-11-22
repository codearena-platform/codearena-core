package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/codearena-platform/codearena-core/internal/runtime"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type Config struct {
	Port    string
	MaxBots int
}

// Start runs the runtime until a termination signal is received.
func Start(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return StartWithContext(ctx, cfg)
}

// StartWithContext drains the gRPC server when ctx is canceled and stops every
// bot container the runtime created before returning.
func StartWithContext(ctx context.Context, cfg Config) error {
	if cfg.Port == "" {
		cfg.Port = "50052"
	}

	slog.Info("CodeArena Runtime Service starting", "port", cfg.Port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer lis.Close()

	authCfg, err := runtime.AuthConfigFromEnv()
	if err != nil {
		return fmt.Errorf("runtime authentication configuration: %w", err)
	}

	runtimeSvc, err := runtime.NewRuntimeService(cfg.MaxBots)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime service: %w", err)
	}
	defer func() {
		containerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		runtimeSvc.Shutdown(containerCtx)
	}()

	grpcServer := grpc.NewServer(authCfg.ServerOptions()...)
	pb.RegisterRuntimeServiceServer(grpcServer, runtimeSvc)
	// Reflection is only enabled for the explicit local-dev insecure opt-out.
	if authCfg.Insecure {
		slog.Warn("Runtime gRPC reflection enabled for local development only")
		reflection.Register(grpcServer)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(lis)
	}()

	slog.Info("Runtime GRPC Server listening", "address", lis.Addr())
	select {
	case <-ctx.Done():
		slog.Info("Runtime shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("failed to serve: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gracefulStop(grpcServer, shutdownCtx)
	return nil
}

// gracefulStop drains in-flight RPCs but force-stops the server if the
// deadline expires, so shutdown can never hang forever.
func gracefulStop(server *grpc.Server, ctx context.Context) {
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		server.Stop()
		<-stopped
	}
}
