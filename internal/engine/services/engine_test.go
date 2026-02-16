package services

import (
	"testing"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestEngine_Tick_HitDetection(t *testing.T) {
	e := NewSimulationEngine(800, 600, nil)
	e.Status = pb.MatchStatus_RUNNING

	// Bot 1 at (100, 100)
	e.SetBot("bot1", &pb.BotState{
		Id:       "bot1",
		Position: &pb.Vector3{X: 100, Y: 100},
		Hull:     100,
	})

	// Bullet coming at Bot 1
	// Heading 180 (South) -> moving towards Y increases
	// Let's place it at (100, 80) moving South with velocity 20
	// In next tick, it will be at (100, 100) -> HIT
	e.Bullets = []*pb.BulletState{
		{
			Id:       "b1",
			OwnerId:  "bot2",
			Position: &pb.Vector3{X: 100, Y: 80},
			Heading:  180,
			Velocity: 20,
			Power:    10,
		},
	}

	state := e.Tick()

	bot1 := e.Bots["bot1"]
	if bot1 == nil {
		t.Fatal("Bot1 missing after tick")
	}

	if bot1.Hull >= 100 {
		t.Errorf("Bot1 should have taken damage, hull: %f", bot1.Hull)
	}

	if len(e.Bullets) != 0 {
		t.Errorf("Bullet should have been removed after hit, count: %d", len(e.Bullets))
	}

	// Check for events
	hitEventFound := false
	for _, ev := range state.Events {
		if ev.GetHitByBullet() != nil {
			hitEventFound = true
			break
		}
	}
	if !hitEventFound {
		t.Error("Expected HitByBullet event, not found")
	}
}

func TestEngine_Tick_Death(t *testing.T) {
	e := NewSimulationEngine(800, 600, nil)
	e.Status = pb.MatchStatus_RUNNING

	// Bot 1 with almost no hull
	e.SetBot("bot1", &pb.BotState{
		Id:       "bot1",
		Position: &pb.Vector3{X: 100, Y: 100},
		Hull:     1,
	})

	// Heavy bullet hit
	e.Bullets = []*pb.BulletState{
		{
			Id:       "b1",
			OwnerId:  "bot2",
			Position: &pb.Vector3{X: 100, Y: 100},
			Heading:  0,
			Velocity: 0,
			Power:    50,
		},
	}

	state := e.Tick()

	if _, ok := e.Bots["bot1"]; ok {
		t.Error("Bot1 should be removed from engine after death")
	}

	deathEventFound := false
	for _, ev := range state.Events {
		if ev.GetDeath() != nil {
			deathEventFound = true
			break
		}
	}
	if !deathEventFound {
		t.Error("Expected Death event, not found")
	}
}
