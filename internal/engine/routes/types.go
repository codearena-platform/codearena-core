package routes

import (
	"sync"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

// BotRuntimeConfig selects the container image used by runtime-managed bots.
// The RegisterBot proto has no image field, so images are deployment config.
type BotRuntimeConfig struct {
	Image            string
	ImagesByLanguage map[string]string
	GameServerURL    string
}

type SimulationServer struct {
	pb.UnimplementedBotServiceServer
	pb.UnimplementedSimulationServiceServer
	pb.UnimplementedMatchServiceServer
	engine            *services.SimulationEngine
	mu                sync.Mutex
	botChannels       map[string]chan *pb.WorldState
	dashboardChannels map[chan *pb.WorldState]bool
	loop              *services.GameLoop
	TickRate          time.Duration
	botRuntime        BotRuntimeConfig
}

func NewSimulationServer(e *services.SimulationEngine, tickRate time.Duration) *SimulationServer {
	return &SimulationServer{
		engine:            e,
		botChannels:       make(map[string]chan *pb.WorldState),
		dashboardChannels: make(map[chan *pb.WorldState]bool),
		TickRate:          tickRate,
	}
}
