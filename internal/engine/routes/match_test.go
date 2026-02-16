package routes

import (
	"context"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestSimulationServer_GetMatchHighlights(t *testing.T) {
	// Setup engine with mock DB if possible, or just a temporary real one
	// Since we use the glebarez/sqlite (pure go), we can use in-memory!
	db, _ := persistence.NewDatabase(":memory:")
	e := services.NewSimulationEngine(800, 600, db)
	s := NewSimulationServer(e, 16)

	matchID := "match-1"
	// Seed some events
	db.SaveEvents([]persistence.EventLog{
		{MatchID: matchID, Tick: 10, Type: "*pb.SimulationEvent_Death", Payload: `{"death":{"bot_id":"bot1","killer_id":"bot2"}}`},
	})

	resp, err := s.GetMatchHighlights(context.Background(), &pb.ReplayRequest{MatchId: matchID})
	if err != nil {
		t.Fatalf("Failed to get highlights: %v", err)
	}

	if len(resp.Moments) != 1 {
		t.Errorf("Expected 1 highlight, got %d", len(resp.Moments))
	}

	if resp.Moments[0].Description != "Bot bot1 was destroyed by bot2" {
		t.Errorf("Unexpected description: %s", resp.Moments[0].Description)
	}
}
