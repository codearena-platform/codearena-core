package persistence

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type Database struct {
	db *gorm.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	if dbPath == "" {
		dbPath = "codearena.db"
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// Auto Migration
	err = db.AutoMigrate(&Match{}, &Bot{}, &EventLog{})
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return &Database{db: db}, nil
}

func (d *Database) CreateMatch(match *Match) error {
	return d.db.Create(match).Error
}

func (d *Database) UpdateMatch(match *Match) error {
	return d.db.Save(match).Error
}

func (d *Database) UpsertBot(bot *Bot) error {
	return d.db.Save(bot).Error
}

func (d *Database) SaveEvent(event *EventLog) error {
	return d.db.Create(event).Error
}

func (d *Database) SaveEvents(events []EventLog) error {
	if len(events) == 0 {
		return nil
	}
	return d.db.Create(&events).Error
}

func (d *Database) GetEvents(matchID string, startTick, endTick int64) ([]EventLog, error) {
	var events []EventLog
	query := d.db.Where("match_id = ?", matchID)
	if startTick > 0 {
		query = query.Where("tick >= ?", startTick)
	}
	if endTick > 0 {
		query = query.Where("tick <= ?", endTick)
	}
	err := query.Order("tick asc").Find(&events).Error
	return events, err
}

func (d *Database) GetHighlights(matchID string) ([]EventLog, error) {
	var events []EventLog
	// Filter for "interesting" events: Deaths and Hits
	// In v1, we check the 'type' string stored in DB
	interestingTypes := []string{"*pb.SimulationEvent_Death", "*pb.SimulationEvent_MatchFinished"}
	err := d.db.Where("match_id = ? AND type IN ?", matchID, interestingTypes).
		Order("tick asc").Find(&events).Error
	return events, err
}

func (d *Database) IncrementBotWin(botID string) error {
	return d.db.Model(&Bot{}).Where("id = ?", botID).UpdateColumn("wins", gorm.Expr("wins + ?", 1)).Error
}

func (d *Database) RecordBotStats(botID string, kills, deaths int) error {
	return d.db.Model(&Bot{}).Where("id = ?", botID).
		Updates(map[string]interface{}{
			"kills":  gorm.Expr("kills + ?", kills),
			"deaths": gorm.Expr("deaths + ?", deaths),
		}).Error
}

func (d *Database) ListMatches() ([]Match, error) {
	var matches []Match
	err := d.db.Order("created_at desc").Find(&matches).Error
	return matches, err
}

func (d *Database) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
