package persistence

import (
	"time"

	"gorm.io/gorm"
)

type Match struct {
	ID          string `gorm:"primaryKey"`
	Status      string
	WinnerID    string
	ArenaWidth  float32
	ArenaHeight float32
	CreatedAt   time.Time
	FinishedAt  *time.Time
	Events      []EventLog `gorm:"foreignKey:MatchID"`
}

type Bot struct {
	ID        string `gorm:"primaryKey"`
	Name      string
	Image     string
	Wins      int
	Kills     int
	Deaths    int
	UpdatedAt time.Time
}

type EventLog struct {
	gorm.Model
	MatchID string `gorm:"index"`
	Tick    int64
	Type    string
	Payload string // JSON
}

// Stable event type identifiers stored in EventLog.Type. Use these constants
// instead of Go type names (which change with the generated protobuf package).
const (
	EventTypeDeath         = "simulation_event_death"
	EventTypeMatchFinished = "simulation_event_match_finished"
)

// HighlightEventTypes are the event types shown in match replays.
var HighlightEventTypes = []string{EventTypeDeath, EventTypeMatchFinished}
