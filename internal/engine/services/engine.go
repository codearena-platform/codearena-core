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

	// Runtime and managedBots are optional: when no runtime client is
	// configured the engine runs with bots that connect directly.
	Runtime     RuntimeClient
	managedBots map[string]string // botID -> containerID
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
		managedBots:  make(map[string]string),
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

// Reset clears per-match state before a new match is created. The caller must
// stop the previous game loop first.
func (e *SimulationEngine) Reset(matchID string, cfg *pb.ArenaConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.MatchID = matchID
	e.Bots = make(map[string]*pb.BotState)
	e.Intents = make(map[string]*pb.BotIntent)
	e.Bullets = make([]*pb.BulletState, 0)
	e.Events = make([]*pb.SimulationEvent, 0)
	e.Zone = nil
	e.CurrentTick = 0
	e.Status = pb.MatchStatus_WAITING
	e.participants = make(map[string]string)
	if cfg != nil {
		e.ArenaConfig = &pb.ArenaConfig{Width: cfg.Width, Height: cfg.Height}
		for _, zone := range cfg.Zones {
			if zone == nil || zone.Position == nil {
				continue
			}
			e.Zone = &pb.ZoneState{
				X: zone.Position.X, Y: zone.Position.Y, Radius: zone.Radius,
			}
			break
		}
	}
}

// GetMatchID returns the active match identifier under the engine lock.
func (e *SimulationEngine) GetMatchID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.MatchID
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

// GetBot returns the bot state registered under id. Callers must not mutate it.
func (e *SimulationEngine) GetBot(id string) *pb.BotState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := e.Bots[id]
	if state == nil {
		return nil
	}
	return state
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
