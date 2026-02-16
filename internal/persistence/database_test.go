package persistence

import (
	"os"
	"testing"
)

func TestDatabase_ReplayFlow(t *testing.T) {
	dbPath := "test_replay.db"
	defer os.Remove(dbPath)

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	matchID := "test-match-1"

	// 1. Save Batch Events
	events := []EventLog{
		{MatchID: matchID, Tick: 10, Type: "Hit", Payload: `{"msg":"hit"}`},
		{MatchID: matchID, Tick: 20, Type: "Death", Payload: `{"msg":"death"}`},
		{MatchID: matchID, Tick: 30, Type: "Finish", Payload: `{"msg":"finish"}`},
		{MatchID: "other", Tick: 5, Type: "Hit", Payload: `{"msg":"other"}`},
	}

	if err := db.SaveEvents(events); err != nil {
		t.Fatalf("Failed to save events: %v", err)
	}

	// 2. Test GetEvents for specific match
	saved, err := db.GetEvents(matchID, 0, 0)
	if err != nil || len(saved) != 3 {
		t.Errorf("Expected 3 events for match, got %d, err: %v", len(saved), err)
	}

	// 3. Test Filtered GetEvents
	filtered, _ := db.GetEvents(matchID, 15, 25)
	if len(filtered) != 1 || filtered[0].Tick != 20 {
		t.Errorf("Expected 1 filtered event at tick 20, got %d", len(filtered))
	}
}

func TestDatabase_Highlights(t *testing.T) {
	dbPath := "test_highlights.db"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)
	defer db.Close()

	matchID := "match-highlight"
	events := []EventLog{
		{MatchID: matchID, Tick: 10, Type: "Hit", Payload: "{}"},
		{MatchID: matchID, Tick: 50, Type: "*pb.SimulationEvent_Death", Payload: "{}"},
		{MatchID: matchID, Tick: 100, Type: "*pb.SimulationEvent_MatchFinished", Payload: "{}"},
	}
	db.SaveEvents(events)

	highlights, err := db.GetHighlights(matchID)
	if err != nil || len(highlights) != 2 {
		t.Errorf("Expected 2 highlights (death, finish), got %d, err: %v", len(highlights), err)
	}
}

func TestDatabase_BotUpsert(t *testing.T) {
	dbPath := "test_bots.db"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)
	defer db.Close()

	bot := &Bot{ID: "bot1", Name: "Alpha", Wins: 0}
	db.UpsertBot(bot)

	db.IncrementBotWin("bot1")

	var saved Bot
	db.db.First(&saved, "id = ?", "bot1")
	if saved.Wins != 1 {
		t.Errorf("Expected 1 win, got %d", saved.Wins)
	}
}
