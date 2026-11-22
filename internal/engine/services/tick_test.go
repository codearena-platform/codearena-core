package services

import (
	"sync"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestEventLogType(t *testing.T) {
	tests := []struct {
		name string
		ev   *pb.SimulationEvent
		want string
	}{
		{name: "death", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_Death{}}, want: persistence.EventTypeDeath},
		{name: "finish", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_MatchFinished{}}, want: persistence.EventTypeMatchFinished},
		{name: "other", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_HitWall{}}, want: "hit_wall"},
		{name: "nil event", ev: nil, want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventLogType(tt.ev); got != tt.want {
				t.Fatalf("eventLogType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTickFinishesWithLastBotStanding(t *testing.T) {
	e := NewSimulationEngine(800, 600, nil)
	e.SetStatus(pb.MatchStatus_RUNNING)
	e.SetBot("bot1", &pb.BotState{Id: "bot1", Name: "one", Hull: 100, Position: &pb.Vector3{X: 100, Y: 100}})
	e.SetBot("bot2", &pb.BotState{Id: "bot2", Name: "two", Hull: 100, Position: &pb.Vector3{X: 100, Y: 120}})
	e.Bullets = []*pb.BulletState{{
		Id:       "bullet-1",
		OwnerId:  "bot1",
		Position: &pb.Vector3{X: 100, Y: 100},
		Heading:  180,
		Velocity: 20,
		Power:    150,
	}}

	state := e.Tick()

	if e.GetStatus() != pb.MatchStatus_FINISHED {
		t.Fatalf("status = %v, want FINISHED", e.GetStatus())
	}
	if got := e.GetAliveCount(); got != 1 {
		t.Fatalf("alive bots = %d, want 1", got)
	}

	var winner, killer string
	for _, ev := range state.Events {
		if death := ev.GetDeath(); death != nil && death.BotId == "bot2" {
			killer = death.KillerId
		}
		if finished := ev.GetMatchFinished(); finished != nil {
			winner = finished.WinnerId
		}
	}
	if winner != "bot1" {
		t.Errorf("winner = %q, want bot1", winner)
	}
	if killer != "bot1" {
		t.Errorf("killer = %q, want bot1", killer)
	}
}

func TestTickDrawWhenAllParticipantsDie(t *testing.T) {
	e := NewSimulationEngine(800, 600, nil)
	e.SetStatus(pb.MatchStatus_RUNNING)
	e.SetBot("bot1", &pb.BotState{Id: "bot1", Hull: 1, Position: &pb.Vector3{X: 100, Y: 100}})
	e.Bullets = []*pb.BulletState{{
		Id:       "bullet-1",
		OwnerId:  "bot2",
		Position: &pb.Vector3{X: 100, Y: 100},
		Heading:  0,
		Velocity: 0,
		Power:    50,
	}}

	state := e.Tick()

	if e.GetStatus() != pb.MatchStatus_FINISHED {
		t.Fatalf("status = %v, want FINISHED", e.GetStatus())
	}
	for _, ev := range state.Events {
		if finished := ev.GetMatchFinished(); finished != nil && finished.WinnerId != "" {
			t.Fatalf("winner = %q, want empty for a draw", finished.WinnerId)
		}
	}
}

// Run with -race: concurrent intents must never race with the tick snapshot.
func TestTickConcurrentIntents(t *testing.T) {
	e := NewSimulationEngine(800, 600, nil)
	e.SetStatus(pb.MatchStatus_RUNNING)
	e.SetBot("bot1", &pb.BotState{Id: "bot1", Hull: 100, Position: &pb.Vector3{X: 100, Y: 100}})
	e.SetBot("bot2", &pb.BotState{Id: "bot2", Hull: 100, Position: &pb.Vector3{X: 300, Y: 300}})

	const writers = 4
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			botID := "bot1"
			if id%2 == 1 {
				botID = "bot2"
			}
			for {
				select {
				case <-stop:
					return
				default:
					e.SetBotIntent(botID, &pb.BotIntent{MoveDistance: float32(id + 1)})
				}
			}
		}(i)
	}

	for i := 0; i < 200; i++ {
		e.Tick()
	}
	close(stop)
	wg.Wait()

	if got := e.GetTick(); got != 200 {
		t.Fatalf("tick = %d, want 200", got)
	}
	if e.GetStatus() != pb.MatchStatus_RUNNING {
		t.Fatalf("status = %v, want RUNNING", e.GetStatus())
	}
}
