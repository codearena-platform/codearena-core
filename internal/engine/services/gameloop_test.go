package services

import (
	"testing"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestGameLoopStopUnblocksRun(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	engine.Status = pb.MatchStatus_RUNNING
	loop := NewGameLoop(engine, time.Millisecond, nil)

	done := make(chan struct{})
	go func() {
		loop.Run()
		close(done)
	}()

	loop.Stop()
	loop.Stop() // repeated stops must not panic on a closed channel

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("game loop did not exit after Stop")
	}
}

func TestGameLoopStoppedBeforeStart(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	loop := NewGameLoop(engine, time.Second, nil)
	loop.Stop()

	done := make(chan struct{})
	go func() {
		loop.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("game loop should exit immediately when stopped before Run")
	}
}

func TestGameLoopStopBeforeStartClosesDone(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	loop := NewGameLoop(engine, time.Millisecond, nil)
	loop.Stop()

	select {
	case <-loop.Done():
	case <-time.After(time.Second):
		t.Fatal("Done was not closed when an unstarted loop was stopped")
	}

	loop.Start() // must not spawn a goroutine after Stop
	if engine.GetStatus() == pb.MatchStatus_RUNNING {
		t.Error("stopped loop was started")
	}
}
