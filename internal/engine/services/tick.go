package services

import (
	"sort"
	"time"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Tick executes a single simulation step. Shared state is read and written
// under the engine lock; physics receives deep snapshots so a concurrent
// SetBotIntent or SetBot can never race with the simulation step.
func (e *SimulationEngine) Tick() *pb.WorldState {
	e.mu.Lock()
	e.CurrentTick++
	tick := e.CurrentTick
	currentState := e.snapshotStateLocked(tick - 1)
	intents := e.snapshotIntentsLocked()
	e.mu.Unlock()

	newState := e.Physics.Update(currentState, e.ArenaConfig, intents)
	e.updateFromState(newState)
	e.checkWinCondition()
	e.persistEvents()

	state := e.GetWorldState()

	e.mu.Lock()
	e.Events = make([]*pb.SimulationEvent, 0)
	e.Intents = make(map[string]*pb.BotIntent)
	e.mu.Unlock()

	return state
}

func (e *SimulationEngine) snapshotStateLocked(tick int64) *pb.WorldState {
	bots := e.getBotSliceInternal()
	clonedBots := make([]*pb.BotState, 0, len(bots))
	for _, bot := range bots {
		if bot != nil {
			clonedBots = append(clonedBots, proto.Clone(bot).(*pb.BotState))
		}
	}

	bullets := make([]*pb.BulletState, 0, len(e.Bullets))
	for _, bullet := range e.Bullets {
		if bullet != nil {
			bullets = append(bullets, proto.Clone(bullet).(*pb.BulletState))
		}
	}

	var zone *pb.ZoneState
	if e.Zone != nil {
		zone = proto.Clone(e.Zone).(*pb.ZoneState)
	}

	return &pb.WorldState{
		Tick:    tick,
		Bots:    clonedBots,
		Bullets: bullets,
		Zone:    zone,
	}
}

func (e *SimulationEngine) snapshotIntentsLocked() map[string]*pb.BotIntent {
	intents := make(map[string]*pb.BotIntent, len(e.Intents))
	for botID, intent := range e.Intents {
		if intent == nil {
			intents[botID] = nil
			continue
		}
		intents[botID] = proto.Clone(intent).(*pb.BotIntent)
	}
	return intents
}

func (e *SimulationEngine) persistEvents() {
	if e.DB == nil {
		return
	}

	e.mu.RLock()
	matchID := e.MatchID
	dbEvents := make([]persistence.EventLog, 0, len(e.Events))
	for _, ev := range e.Events {
		if ev == nil {
			continue
		}
		payload, _ := protojson.Marshal(ev)
		dbEvents = append(dbEvents, persistence.EventLog{
			MatchID: matchID,
			Tick:    ev.Tick,
			Type:    eventLogType(ev),
			Payload: string(payload),
		})
	}
	e.mu.RUnlock()

	if len(dbEvents) == 0 {
		return
	}
	if matchID == "" {
		matchID = "default-match"
		for i := range dbEvents {
			dbEvents[i].MatchID = matchID
		}
	}
	_ = e.DB.SaveEvents(dbEvents)
}

// eventLogType returns a stable identifier for a simulated event. Go type names
// are not usable here: the generated protobuf package name changes the output of
// %T and breaks replay highlight queries.
func eventLogType(ev *pb.SimulationEvent) string {
	if ev == nil {
		return "unknown"
	}
	switch ev.Event.(type) {
	case *pb.SimulationEvent_Death:
		return persistence.EventTypeDeath
	case *pb.SimulationEvent_MatchFinished:
		return persistence.EventTypeMatchFinished
	case *pb.SimulationEvent_HitByBullet:
		return "hit_by_bullet"
	case *pb.SimulationEvent_BulletHitTarget:
		return "bullet_hit_target"
	case *pb.SimulationEvent_HitWall:
		return "hit_wall"
	case *pb.SimulationEvent_HitRobot:
		return "hit_robot"
	case *pb.SimulationEvent_ZoneEntered:
		return "zone_entered"
	default:
		return "unknown"
	}
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
		if b == nil {
			continue
		}
		aliveBots[b.Id] = true
		e.Bots[b.Id] = b // Update existing/new bots
		name := b.Name
		if name == "" {
			name = b.Id
		}
		e.participants[b.Id] = name
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

type botResult struct {
	name   string
	kills  int
	deaths int
	wins   int
}

func (e *SimulationEngine) sortedParticipantsLocked() []string {
	ids := make([]string, 0, len(e.participants))
	for id := range e.participants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// determineWinnerLocked finishes a match as soon as one bot survives. If every
// participant died, the bot with the most recorded kills wins; a draw has no
// winner. It returns false while the match has no entrants or several survivors.
func (e *SimulationEngine) determineWinnerLocked() (string, bool) {
	alive := make([]string, 0, len(e.Bots))
	for id := range e.Bots {
		alive = append(alive, id)
	}
	sort.Strings(alive)

	switch len(alive) {
	case 1:
		return alive[0], true
	case 0:
		if len(e.participants) == 0 {
			return "", false
		}
		kills := map[string]int{}
		for _, ev := range e.Events {
			if death := ev.GetDeath(); death != nil && death.KillerId != "" {
				kills[death.KillerId]++
			}
		}
		winner := ""
		best := 0
		for _, id := range e.sortedParticipantsLocked() {
			if kills[id] > best {
				best = kills[id]
				winner = id
			}
		}
		return winner, true
	default:
		return "", false
	}
}

func (e *SimulationEngine) botResultsLocked(winnerID string) map[string]botResult {
	results := make(map[string]botResult, len(e.participants))
	for id, name := range e.participants {
		results[id] = botResult{name: name}
	}
	for _, ev := range e.Events {
		death := ev.GetDeath()
		if death == nil {
			continue
		}
		if result, ok := results[death.BotId]; ok {
			result.deaths++
			results[death.BotId] = result
		}
		if death.KillerId != "" {
			if result, ok := results[death.KillerId]; ok {
				result.kills++
				results[death.KillerId] = result
			}
		}
	}
	if result, ok := results[winnerID]; ok {
		result.wins++
		results[winnerID] = result
	}
	return results
}

func (e *SimulationEngine) checkWinCondition() {
	e.mu.Lock()
	if e.Status != pb.MatchStatus_RUNNING {
		e.mu.Unlock()
		return
	}
	winnerID, finished := e.determineWinnerLocked()
	if !finished {
		e.mu.Unlock()
		return
	}

	now := time.Now()
	matchID := e.MatchID
	if matchID == "" {
		matchID = "match-" + now.Format("20060102-150405")
	}
	e.Status = pb.MatchStatus_FINISHED
	e.Events = append(e.Events, &pb.SimulationEvent{
		Tick: e.CurrentTick,
		Event: &pb.SimulationEvent_MatchFinished{
			MatchFinished: &pb.MatchFinishedEvent{WinnerId: winnerID},
		},
	})
	match := &persistence.Match{
		ID:          matchID,
		Status:      "FINISHED",
		WinnerID:    winnerID,
		ArenaWidth:  e.ArenaConfig.Width,
		ArenaHeight: e.ArenaConfig.Height,
		CreatedAt:   now.Add(-time.Duration(e.CurrentTick) * 16 * time.Millisecond),
		FinishedAt:  &now,
	}
	results := e.botResultsLocked(winnerID)
	participantIDs := e.sortedParticipantsLocked()
	e.mu.Unlock()

	if e.DB == nil {
		return
	}
	_ = e.DB.CreateMatch(match)
	for _, id := range participantIDs {
		result := results[id]
		name := result.name
		if name == "" {
			name = id
		}
		_ = e.DB.UpsertBot(&persistence.Bot{
			ID:     id,
			Name:   name,
			Image:  "unknown",
			Wins:   result.wins,
			Kills:  result.kills,
			Deaths: result.deaths,
		})
	}
}
