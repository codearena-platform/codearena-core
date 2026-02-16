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
