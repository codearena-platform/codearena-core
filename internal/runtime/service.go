package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/codearena-platform/codearena-core/internal/runtime/docker"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type BotRunner interface {
	StartContainer(ctx context.Context, image string, botID string, env []string) (string, error)
	StopContainer(ctx context.Context, containerID string) error
	CountActiveContainers(ctx context.Context) (int, error)
}

type RuntimeService struct {
	pb.UnimplementedRuntimeServiceServer
	runner    BotRunner
	scheduler *Scheduler
}

func NewRuntimeService(maxBots int) (*RuntimeService, error) {
	runner, err := docker.NewBotRunner()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker runner: %w", err)
	}
	scheduler := NewScheduler(runner, maxBots)
	return &RuntimeService{runner: runner, scheduler: scheduler}, nil
}

func NewRuntimeServiceWithRunner(runner BotRunner, maxBots int) *RuntimeService {
	scheduler := NewScheduler(runner, maxBots)
	return &RuntimeService{runner: runner, scheduler: scheduler}
}

func (s *RuntimeService) StartBot(ctx context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error) {
	slog.Info("Request to start bot", "bot_id", req.BotId, "image", req.Image, "match_id", req.MatchId)

	env := []string{
		fmt.Sprintf("GAME_SERVER_URL=%s", req.GameServerUrl),
		fmt.Sprintf("BOT_ID=%s", req.BotId),
		fmt.Sprintf("MATCH_ID=%s", req.MatchId),
	}

	containerID, queued, pos, err := s.scheduler.StartBot(ctx, req.BotId, req.Image, req.MatchId, env)
	if err != nil {
		slog.Error("Error starting bot", "bot_id", req.BotId, "match_id", req.MatchId, "error", err)
		return &pb.StartBotResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	if queued {
		return &pb.StartBotResponse{
			Success:       true,
			Queued:        true,
			QueuePosition: int32(pos),
		}, nil
	}

	slog.Info("Bot started successfully", "bot_id", req.BotId, "container_id", containerID, "match_id", req.MatchId)
	return &pb.StartBotResponse{
		Success:     true,
		ContainerId: containerID,
	}, nil
}

func (s *RuntimeService) StopBot(ctx context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error) {
	if req.ContainerId == "" {
		return &pb.StopBotResponse{Success: false}, fmt.Errorf("container_id is required")
	}

	err := s.runner.StopContainer(ctx, req.ContainerId)
	if err != nil {
		slog.Error("Error stopping container", "container_id", req.ContainerId, "error", err)
		return &pb.StopBotResponse{Success: false}, nil
	}

	// Notify scheduler that a slot is free
	if req.BotId != "" {
		s.scheduler.NotifyStop(req.BotId)
	}

	return &pb.StopBotResponse{Success: true}, nil
}

func (s *RuntimeService) GetRuntimeStats(ctx context.Context, req *pb.Empty) (*pb.RuntimeStats, error) {
	active := s.scheduler.GetActiveCount()
	return &pb.RuntimeStats{
		ActiveContainers: int32(active),
		// Memory/CPU usage requires more complex Docker inspection, leaving as 0 for now
		MemoryUsageMb:   0,
		CpuUsagePercent: 0.0,
	}, nil
}
