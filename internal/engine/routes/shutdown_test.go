package routes

import (
	"context"
	"testing"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestSimulationServerShutdownStopsLoopAndClosesChannels(t *testing.T) {
	engine := services.NewSimulationEngine(800, 600, nil)
	server := NewSimulationServer(engine, time.Millisecond)

	loop := services.NewGameLoop(engine, time.Millisecond, nil)
	server.loop = loop
	engine.SetStatus(pb.MatchStatus_RUNNING)
	loop.Start()

	botCh := make(chan *pb.WorldState, 1)
	botCh <- &pb.WorldState{Tick: 1}
	server.botChannels["bot-1"] = botCh
	dashCh := make(chan *pb.WorldState, 1)
	server.dashboardChannels[dashCh] = true

	server.Shutdown()

	select {
	case <-loop.Done():
	case <-time.After(time.Second):
		t.Fatal("game loop did not exit before Shutdown returned")
	}
	if server.loop != nil {
		t.Error("server still holds a loop after Shutdown")
	}
	if len(server.botChannels) != 0 || len(server.dashboardChannels) != 0 {
		t.Errorf("stream channels remain registered: bots=%d dashboards=%d",
			len(server.botChannels), len(server.dashboardChannels))
	}
	// Drain the buffered world state first; a closed channel still yields
	// buffered values before reporting ok=false.
	<-botCh
	if _, ok := <-botCh; ok {
		t.Error("bot stream channel was not closed")
	}
	if _, ok := <-dashCh; ok {
		t.Error("dashboard stream channel was not closed")
	}
}

func TestServersShutdownBeforeServeDoesNotBind(t *testing.T) {
	servers := NewServers(services.NewSimulationEngine(800, 600, nil), time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := servers.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- servers.Serve("127.0.0.1:0", "127.0.0.1:0")
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after Shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve blocked after Shutdown")
	}
}

func TestServersServeCanBeShutDownGracefully(t *testing.T) {
	servers := NewServers(services.NewSimulationEngine(800, 600, nil), time.Millisecond)
	done := make(chan error, 1)
	go func() {
		done <- servers.Serve("127.0.0.1:0", "127.0.0.1:0")
	}()

	// Give the listeners a moment to bind before draining them.
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := servers.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned an error during graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
}
