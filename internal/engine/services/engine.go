package services

import (
	"sync"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type SimulationEngine struct {
	MatchID     string
	ArenaConfig *pb.ArenaConfig
	Bots        map[string]*pb.BotState
	Intents     map[string]*pb.BotIntent
	mu          sync.RWMutex
	Bullets     []*pb.BulletState
	Events      []*pb.SimulationEvent
	Zone        *pb.ZoneState
	CurrentTick int64
	Status      pb.MatchStatus
	Physics     *PhysicsEngine
	DB          *persistence.Database
}

func NewSimulationEngine(width, height float32, db *persistence.Database) *SimulationEngine {
	return &SimulationEngine{
		ArenaConfig: &pb.ArenaConfig{
			Width:  width,
			Height: height,
		},
		Bots:    make(map[string]*pb.BotState),
		Intents: make(map[string]*pb.BotIntent),
		Bullets: make([]*pb.BulletState, 0),
		Events:  make([]*pb.SimulationEvent, 0),
		Status:  pb.MatchStatus_WAITING,
		Physics: NewPhysicsEngine(),
		DB:      db,
	}
}

func (e *SimulationEngine) SetBotIntent(id string, intent *pb.BotIntent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Intents[id] = intent
}

func (e *SimulationEngine) SetBot(id string, state *pb.BotState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Bots[id] = state
}
