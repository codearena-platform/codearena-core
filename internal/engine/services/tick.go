package services

import (
	"fmt"
	"sort"
	"time"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// Tick executes a single simulation step
func (e *SimulationEngine) Tick() *pb.WorldState {
	e.CurrentTick++

	e.mu.RLock()
	// 1. Prepare Current World State Snapshot for Physics
	currentState := &pb.WorldState{
		Tick:    e.CurrentTick - 1,
		Bots:    e.getBotSliceInternal(),
		Bullets: e.Bullets,
		Zone:    e.Zone,
		Events:  nil,
	}
	e.mu.RUnlock()

	// 2. Physics Update
	newState := e.Physics.Update(currentState, e.ArenaConfig, e.Intents)

	// 3. Update Engine State
	e.updateFromState(newState)

	// 4. Handle Higher Level Game Logic
	e.checkWinCondition()

	// 6. Persist Events to DB
	if e.DB != nil && len(e.Events) > 0 {
		matchID := e.MatchID
		if matchID == "" {
			matchID = "default-match"
		}
		dbEvents := make([]persistence.EventLog, 0, len(e.Events))
		for _, ev := range e.Events {
			payload, _ := protojson.Marshal(ev)
			dbEvents = append(dbEvents, persistence.EventLog{
				MatchID: matchID,
				Tick:    ev.Tick,
				Type:    fmt.Sprintf("%T", ev.Event),
				Payload: string(payload),
			})
		}
		e.DB.SaveEvents(dbEvents)
	}

	// 7. Get final state for return
	state := e.GetWorldState()

	// 8. Clear events and intents for next tick
	e.mu.Lock()
	e.Events = make([]*pb.SimulationEvent, 0)
	e.Intents = make(map[string]*pb.BotIntent)
	e.mu.Unlock()

	return state
}

func (e *SimulationEngine) getBotSliceInternal() []*pb.BotState {
	// Sort IDs for determinism (Go map iteration is random)
	ids := make([]string, 0, len(e.Bots))
	for id := range e.Bots {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	slice := make([]*pb.BotState, 0, len(e.Bots))
	for _, id := range ids {
		slice = append(slice, e.Bots[id])
	}
	return slice
}

func (e *SimulationEngine) GetBotSlice() []*pb.BotState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.getBotSliceInternal()
}

func (e *SimulationEngine) updateFromState(newState *pb.WorldState) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Create a set of alive bot IDs for reconciliation
	aliveBots := make(map[string]bool)
	for _, b := range newState.Bots {
		aliveBots[b.Id] = true
		e.Bots[b.Id] = b // Update existing/new bots
	}

	// 2. Remove bots that are no longer in the physics state
	for id := range e.Bots {
		if !aliveBots[id] {
			delete(e.Bots, id)
		}
	}

	e.Bullets = newState.Bullets
	e.Zone = newState.Zone

	if len(newState.Events) > 0 {
		e.Events = append(e.Events, newState.Events...)
	}
}

func (e *SimulationEngine) GetWorldState() *pb.WorldState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return &pb.WorldState{
		Tick:    e.CurrentTick,
		Status:  e.Status,
		Bots:    e.getBotSliceInternal(),
		Bullets: e.Bullets,
		Zone:    e.Zone,
		Events:  e.Events,
	}
}

func (e *SimulationEngine) checkWinCondition() {
	if e.Status != pb.MatchStatus_RUNNING {
		return
	}

	aliveCount := len(e.Bots)
	if aliveCount == 0 && e.CurrentTick > 1000 {
		e.Status = pb.MatchStatus_FINISHED
		now := time.Now()

		matchID := e.MatchID
		if matchID == "" {
			matchID = "match-" + now.Format("20060102-150405")
		}

		e.Events = append(e.Events, &pb.SimulationEvent{
			Tick: e.CurrentTick,
			Event: &pb.SimulationEvent_MatchFinished{
				MatchFinished: &pb.MatchFinishedEvent{},
			},
		})

		// Persistence Logic
		if e.DB != nil {
			match := &persistence.Match{
				ID:          matchID,
				Status:      "FINISHED",
				ArenaWidth:  e.ArenaConfig.Width,
				ArenaHeight: e.ArenaConfig.Height,
				CreatedAt:   now.Add(-time.Duration(e.CurrentTick) * 16 * time.Millisecond),
				FinishedAt:  &now,
			}
			e.DB.CreateMatch(match)

			// Record stats for all participating bots (even dead ones)
			// Assuming e.Bots only contains ALIVE bots.
			// We need to track all bots that were in the match.
			// For now, let's just record for the bots currently in the map.
			for id, bot := range e.Bots {
				e.DB.UpsertBot(&persistence.Bot{
					ID:    id,
					Name:  bot.Id, // Fallback
					Image: "unknown",
				})
				// If aliveCount was 1, we could determine a winner.
			}
		}
	}
}
