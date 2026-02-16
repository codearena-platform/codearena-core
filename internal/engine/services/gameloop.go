package services

import (
	"log/slog"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type GameLoop struct {
	Engine    *SimulationEngine
	Ticker    *time.Ticker
	StopChan  chan bool
	Broadcast func(*pb.WorldState)
	TickRate  time.Duration
}

func NewGameLoop(engine *SimulationEngine, tickRate time.Duration, broadcast func(*pb.WorldState)) *GameLoop {
	if tickRate == 0 {
		tickRate = 16 * time.Millisecond // Default to ~60 TPS
	}
	return &GameLoop{
		Engine:    engine,
		Broadcast: broadcast,
		StopChan:  make(chan bool),
		TickRate:  tickRate,
	}
}

func (gl *GameLoop) Run() {
	gl.Ticker = time.NewTicker(gl.TickRate)
	defer gl.Ticker.Stop()

	for {
		select {
		case <-gl.StopChan:
			return
		case <-gl.Ticker.C:
			if gl.Engine.Status == pb.MatchStatus_RUNNING {
				if gl.Engine.CurrentTick%60 == 0 {
					slog.Debug("GameLoop Tick", "tick", gl.Engine.CurrentTick, "bots_count", len(gl.Engine.Bots))
				}
				state := gl.Engine.Tick()

				// Broadcast state
				if gl.Broadcast != nil {
					gl.Broadcast(state)
				}

				if gl.Engine.Status == pb.MatchStatus_FINISHED {
					return
				}
			}
		}
	}
}

func (gl *GameLoop) Start() {
	gl.Engine.Status = pb.MatchStatus_RUNNING
	go gl.Run()
}

func (gl *GameLoop) Stop() {
	go func() {
		gl.StopChan <- true
	}()
}
