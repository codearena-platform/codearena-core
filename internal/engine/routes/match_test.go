package routes

import (
	"context"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestSimulationServer_GetMatchReplayFiltersTicksAndMalformedEvents(t *testing.T) {
	db, err := persistence.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	events := []persistence.EventLog{
		{MatchID: "match-1", Tick: 1, Type: "test", Payload: `{"tick":1}`},
		{MatchID: "match-1", Tick: 5, Type: "test", Payload: `{"tick":5}`},
		{MatchID: "match-1", Tick: 6, Type: "test", Payload: "not-json"},
		{MatchID: "match-1", Tick: 10, Type: "test", Payload: `{"tick":10}`},
		{MatchID: "other", Tick: 5, Type: "test", Payload: `{"tick":5}`},
	}
	if err := db.SaveEvents(events); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	server := NewSimulationServer(services.NewSimulationEngine(800, 600, db), 16)
	resp, err := server.GetMatchReplay(context.Background(), &pb.ReplayRequest{
		MatchId: "match-1", StartTick: 5, EndTick: 6,
	})
	if err != nil {
		t.Fatalf("GetMatchReplay: %v", err)
	}
	if resp.MatchId != "match-1" || len(resp.Events) != 1 || resp.Events[0].Tick != 5 {
		t.Fatalf("unexpected replay: %+v", resp)
	}
}

func TestSimulationServer_ListMatchesMapsStatus(t *testing.T) {
	db, err := persistence.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.CreateMatch(&persistence.Match{ID: "running", Status: "RUNNING"}); err != nil {
		t.Fatalf("CreateMatch(running): %v", err)
	}
	if err := db.CreateMatch(&persistence.Match{ID: "finished", Status: "FINISHED"}); err != nil {
		t.Fatalf("CreateMatch(finished): %v", err)
	}

	server := NewSimulationServer(services.NewSimulationEngine(800, 600, db), 16)
	resp, err := server.ListMatches(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	statuses := map[string]pb.MatchStatus{}
	for _, match := range resp.Matches {
		statuses[match.MatchId] = match.Status
	}
	if statuses["running"] != pb.MatchStatus_RUNNING || statuses["finished"] != pb.MatchStatus_FINISHED {
		t.Errorf("unexpected statuses: %+v", statuses)
	}

	active, err := server.ListActiveMatches(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListActiveMatches: %v", err)
	}
	if len(active.Matches) != 0 {
		t.Errorf("no in-process match should be active: %+v", active.Matches)
	}
	server.engine.SetStatus(pb.MatchStatus_RUNNING)
	active, err = server.ListActiveMatches(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListActiveMatches after start: %v", err)
	}
	if len(active.Matches) != 1 {
		t.Errorf("active matches = %+v, want one", active.Matches)
	}
}

func TestSimulationServer_GetMatchHighlights(t *testing.T) {
	// Setup engine with mock DB if possible, or just a temporary real one
	// Since we use the glebarez/sqlite (pure go), we can use in-memory!
	db, _ := persistence.NewDatabase(":memory:")
	e := services.NewSimulationEngine(800, 600, db)
	s := NewSimulationServer(e, 16)

	matchID := "match-1"
	// Seed some events
	db.SaveEvents([]persistence.EventLog{
		{MatchID: matchID, Tick: 10, Type: persistence.EventTypeDeath, Payload: `{"death":{"bot_id":"bot1","killer_id":"bot2"}}`},
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
