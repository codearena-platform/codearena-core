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

	// participants keeps names after a bot is removed from Bots on death so
	// match results can still record every entrant.
	participants map[string]string
}

func NewSimulationEngine(width, height float32, db *persistence.Database) *SimulationEngine {
	return &SimulationEngine{
		ArenaConfig: &pb.ArenaConfig{
			Width:  width,
			Height: height,
		},
		Bots:         make(map[string]*pb.BotState),
		Intents:      make(map[string]*pb.BotIntent),
		Bullets:      make([]*pb.BulletState, 0),
		Events:       make([]*pb.SimulationEvent, 0),
		Status:       pb.MatchStatus_WAITING,
		Physics:      NewPhysicsEngine(),
		DB:           db,
		participants: make(map[string]string),
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
	name := id
	if state != nil && state.Name != "" {
		name = state.Name
	}
	e.participants[id] = name
}

// GetStatus returns the match status without racing with the tick loop.
func (e *SimulationEngine) GetStatus() pb.MatchStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Status
}

// SetStatus updates the match status under the engine lock.
func (e *SimulationEngine) SetStatus(status pb.MatchStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Status = status
}

// GetTick returns the current tick under the engine lock.
func (e *SimulationEngine) GetTick() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.CurrentTick
}

// GetAliveCount returns the number of surviving bots under the engine lock.
func (e *SimulationEngine) GetAliveCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.Bots)
}

// IsFinished reports whether the match reached a terminal state.
func (e *SimulationEngine) IsFinished() bool {
	status := e.GetStatus()
	return status == pb.MatchStatus_FINISHED
}
