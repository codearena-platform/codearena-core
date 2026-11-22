package services

import (
	"log/slog"
	"sync"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type GameLoop struct {
	Engine    *SimulationEngine
	Ticker    *time.Ticker
	StopChan  chan struct{}
	Broadcast func(*pb.WorldState)
	TickRate  time.Duration
	stopOnce  sync.Once
	startOnce sync.Once
	stateMu   sync.Mutex
	started   bool
	done      chan struct{}
	doneOnce  sync.Once
}

func NewGameLoop(engine *SimulationEngine, tickRate time.Duration, broadcast func(*pb.WorldState)) *GameLoop {
	if tickRate == 0 {
		tickRate = 16 * time.Millisecond // Default to ~60 TPS
	}
	return &GameLoop{
		Engine:    engine,
		Broadcast: broadcast,
		StopChan:  make(chan struct{}),
		TickRate:  tickRate,
		done:      make(chan struct{}),
	}
}

func (gl *GameLoop) Run() {
	gl.Ticker = time.NewTicker(gl.TickRate)
	defer gl.Ticker.Stop()
	defer gl.doneOnce.Do(func() { close(gl.done) })

	for {
		select {
		case <-gl.StopChan:
			return
		case <-gl.Ticker.C:
			if gl.Engine.GetStatus() == pb.MatchStatus_RUNNING {
				if tick := gl.Engine.GetTick(); tick%60 == 0 {
					slog.Debug("GameLoop Tick", "tick", tick, "bots_count", gl.Engine.GetAliveCount())
				}
				state := gl.Engine.Tick()

				// Broadcast state
				if gl.Broadcast != nil {
					gl.Broadcast(state)
				}

				if gl.Engine.IsFinished() {
					return
				}
			}
		}
	}
}

func (gl *GameLoop) Start() {
	gl.startOnce.Do(func() {
		gl.stateMu.Lock()
		defer gl.stateMu.Unlock()
		select {
		case <-gl.StopChan:
			// Stop was requested before Start; do not spawn a doomed goroutine.
			return
		default:
		}
		gl.started = true
		gl.Engine.SetStatus(pb.MatchStatus_RUNNING)
		go gl.Run()
	})
}

// Stop closes the stop channel so every waiter is released immediately and no
// goroutine is left blocked on a send to a channel nobody reads anymore. It also
// closes Done when the loop was never started, so shutdown cannot hang; when
// the loop is running, Done is closed by Run after the last tick.
func (gl *GameLoop) Stop() {
	gl.stateMu.Lock()
	defer gl.stateMu.Unlock()
	gl.stopOnce.Do(func() {
		close(gl.StopChan)
		if !gl.started {
			gl.doneOnce.Do(func() { close(gl.done) })
		}
	})
}

// Done is closed when Run returns. Shutdown uses it to make sure no tick can
// still be in flight before the engine database is closed.
func (gl *GameLoop) Done() <-chan struct{} {
	return gl.done
}
