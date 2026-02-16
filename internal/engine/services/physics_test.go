package services

import (
	"testing"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestUpdateRobot_Movement(t *testing.T) {
	pe := NewPhysicsEngine()
	arena := &pb.ArenaConfig{Width: 800, Height: 600}
	robot := &pb.BotState{
		Id:       "bot1",
		Position: &pb.Vector3{X: 100, Y: 100},
		Heading:  0, // North (0 degrees)
		Velocity: 0,
		Energy:   100,
	}

	// 1. Accelerate
	intent := &pb.BotIntent{MoveDistance: 100}
	updated := pe.updateRobot(robot, arena, intent)

	if updated.Velocity <= 0 {
		t.Errorf("Expected velocity to increase, got %f", updated.Velocity)
	}

	// Heading 0 (North) -> subtract from Y
	if updated.Position.Y >= 100 {
		t.Errorf("Expected Y to decrease when moving North, got %f", updated.Position.Y)
	}
}

func TestUpdateRobot_BoundaryCollision(t *testing.T) {
	pe := NewPhysicsEngine()
	arena := &pb.ArenaConfig{Width: 800, Height: 600}

	// Near West wall
	robot := &pb.BotState{
		Id:       "bot1",
		Position: &pb.Vector3{X: 25, Y: 100},
		Heading:  270, // West
		Velocity: 10,
		Energy:   100,
	}

	intent := &pb.BotIntent{MoveDistance: 100}
	updated := pe.updateRobot(robot, arena, intent)

	if updated.Position.X < RobotRadius {
		t.Errorf("Bot moved out of west boundary: %f", updated.Position.X)
	}
	if updated.Velocity != 0 {
		t.Errorf("Expected velocity to be 0 after hitting wall, got %f", updated.Velocity)
	}
}

func TestPhysicsUpdate_RobotCollision(t *testing.T) {
	pe := NewPhysicsEngine()
	arena := &pb.ArenaConfig{Width: 800, Height: 600}

	state := &pb.WorldState{
		Tick: 1,
		Bots: []*pb.BotState{
			{Id: "bot1", Position: &pb.Vector3{X: 100, Y: 100}, Hull: 100},
			{Id: "bot2", Position: &pb.Vector3{X: 110, Y: 100}, Hull: 100}, // Overlapping (dist 10 < 40 radius sum)
		},
	}

	newState := pe.Update(state, arena, make(map[string]*pb.BotIntent))

	// Find bots by ID
	var b1, b2 *pb.BotState
	for _, b := range newState.Bots {
		if b.Id == "bot1" {
			b1 = b
		} else if b.Id == "bot2" {
			b2 = b
		}
	}

	if b1 == nil || b2 == nil {
		t.Fatalf("Bots not found in result: bot1=%v, bot2=%v", b1, b2)
	}

	// Bot1 (100) vs Bot2 (110)
	// Bot1 should move West (X < 100), Bot2 should move East (X > 110)
	if b1.Position.X >= 100 {
		t.Errorf("Bot1 (West) should have been pushed further West, got X=%f", b1.Position.X)
	}
	if b2.Position.X <= 110 {
		t.Errorf("Bot2 (East) should have been pushed further East, got X=%f", b2.Position.X)
	}

	if b1.Hull >= 100 || b2.Hull >= 100 {
		t.Errorf("Bots should have taken collision damage: b1.Hull=%f, b2.Hull=%f", b1.Hull, b2.Hull)
	}
}
