package routes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	waitingStatus  = "WAITING"
	runningStatus  = "RUNNING"
	finishedStatus = "FINISHED"
)

// SetBotRuntimeConfig configures optional runtime-managed bots. It is safe to
// call before the server starts serving.
func (s *SimulationServer) SetBotRuntimeConfig(cfg BotRuntimeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.botRuntime = cfg
}

func (s *SimulationServer) botRuntimeSnapshot() BotRuntimeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.botRuntime
}

func botImageFromConfig(cfg BotRuntimeConfig, language string) string {
	key := strings.ToLower(strings.TrimSpace(language))
	if image := cfg.ImagesByLanguage[key]; image != "" {
		return image
	}
	return cfg.Image
}

// CreateMatch resets the single in-process simulation to a fresh match and
// persists it when a database is configured. A running match must be stopped
// first so callers cannot silently wipe live state.
func (s *SimulationServer) CreateMatch(ctx context.Context, cfg *pb.ArenaConfig) (*pb.MatchResponse, error) {
	if cfg == nil {
		return nil, grpcstatus.Error(codes.InvalidArgument, "arena config is required")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, grpcstatus.Errorf(codes.InvalidArgument, "arena dimensions must be positive (got %gx%g)", cfg.Width, cfg.Height)
	}
	if cfg.MaxBots < 0 || cfg.MatchDurationTicks < 0 {
		return nil, grpcstatus.Error(codes.InvalidArgument, "max_bots and match_duration_ticks cannot be negative")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.engine.GetStatus() == pb.MatchStatus_RUNNING {
		return nil, grpcstatus.Error(codes.FailedPrecondition, "stop the running simulation before creating a new match")
	}

	matchID := strings.TrimSpace(cfg.Id)
	if matchID == "" {
		var err error
		matchID, err = newID("match")
		if err != nil {
			return nil, grpcstatus.Error(codes.Internal, err.Error())
		}
	}

	if db := s.engine.DB; db != nil {
		err := db.CreateMatch(&persistence.Match{
			ID:          matchID,
			Status:      waitingStatus,
			ArenaWidth:  cfg.Width,
			ArenaHeight: cfg.Height,
			CreatedAt:   time.Now().UTC(),
		})
		if err != nil {
			return nil, grpcstatus.Errorf(codes.Internal, "persist match: %v", err)
		}
	}

	botCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.engine.StopManagedBots(botCtx)
	s.engine.Reset(matchID, cfg)

	return &pb.MatchResponse{MatchId: matchID, Status: pb.MatchStatus_WAITING}, nil
}

// RegisterBot records a bot for the active match. If a bot image is configured
// for the language, it also asks the runtime to sandbox it; without an image
// the caller is expected to connect a bot directly through BotService.
func (s *SimulationServer) RegisterBot(ctx context.Context, req *pb.RegisterBotRequest) (*pb.RegisterBotResponse, error) {
	if req == nil {
		return nil, grpcstatus.Error(codes.InvalidArgument, "register bot request is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "bot name is required")
	}
	if strings.TrimSpace(req.UserId) == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "user_id is required")
	}

	s.mu.Lock()
	if s.engine.GetStatus() == pb.MatchStatus_RUNNING {
		s.mu.Unlock()
		return nil, grpcstatus.Error(codes.FailedPrecondition, "cannot register bots while a simulation is running")
	}
	matchID := s.engine.GetMatchID()
	position := 100 + 10*len(s.botChannels)
	s.mu.Unlock()

	if matchID == "" {
		return nil, grpcstatus.Error(codes.FailedPrecondition, "create a match before registering bots")
	}

	botID, err := newID("bot")
	if err != nil {
		return nil, grpcstatus.Error(codes.Internal, err.Error())
	}
	versionID, err := newID("ver")
	if err != nil {
		return nil, grpcstatus.Error(codes.Internal, err.Error())
	}

	runtimeCfg := s.botRuntimeSnapshot()
	image := botImageFromConfig(runtimeCfg, req.Language)
	message := "bot registered for direct connection"
	if image != "" {
		if _, err := s.engine.StartManagedBot(ctx, botID, image, matchID, runtimeCfg.GameServerURL); err != nil {
			return nil, grpcstatus.Error(codes.Internal, err.Error())
		}
		message = "bot registered and runtime container requested"
	}

	if db := s.engine.DB; db != nil {
		if err := db.UpsertBot(&persistence.Bot{
			ID:        botID,
			Name:      req.Name,
			Image:     image,
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			// Do not leave a container running for a bot that failed to persist.
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			_ = s.engine.StopManagedBot(stopCtx, botID)
			return nil, grpcstatus.Errorf(codes.Internal, "persist bot: %v", err)
		}
	}

	s.engine.SetBot(botID, &pb.BotState{
		Id:       botID,
		Name:     req.Name,
		Position: &pb.Vector3{X: float32(position), Y: 100},
		Hull:     100,
		Energy:   150,
	})

	return &pb.RegisterBotResponse{
		BotId:     botID,
		VersionId: versionID,
		Success:   true,
		Message:   message,
	}, nil
}

func newID(prefix string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf)), nil
}
