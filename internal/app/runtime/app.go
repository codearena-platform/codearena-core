package runtime

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/codearena-platform/codearena-core/internal/runtime"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type Config struct {
	Port    string
	MaxBots int
}

func Start(cfg Config) error {
	if cfg.Port == "" {
		cfg.Port = "50052"
	}

	slog.Info("CodeArena Runtime Service starting", "port", cfg.Port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	runtimeSvc, err := runtime.NewRuntimeService(cfg.MaxBots)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime service: %w", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRuntimeServiceServer(grpcServer, runtimeSvc)
	reflection.Register(grpcServer)

	slog.Info("Runtime GRPC Server listening", "address", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}
