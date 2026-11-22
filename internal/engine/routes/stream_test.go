package routes

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc/metadata"
)

type fakeBotStream struct {
	ctx  context.Context
	recv chan *pb.BotIntent
	sent chan *pb.WorldState
}

func (s *fakeBotStream) Recv() (*pb.BotIntent, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case in := <-s.recv:
		return in, nil
	}
}

func (s *fakeBotStream) Send(state *pb.WorldState) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- state:
		return nil
	}
}

func (s *fakeBotStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeBotStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeBotStream) SetTrailer(metadata.MD)       {}
func (s *fakeBotStream) Context() context.Context     { return s.ctx }
func (s *fakeBotStream) SendMsg(any) error            { return errors.New("not implemented") }
func (s *fakeBotStream) RecvMsg(any) error            { return errors.New("not implemented") }

type fakeWatchStream struct {
	ctx  context.Context
	sent chan *pb.WorldState
}

func (s *fakeWatchStream) Send(state *pb.WorldState) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- state:
		return nil
	}
}

func (s *fakeWatchStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeWatchStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeWatchStream) SetTrailer(metadata.MD)       {}
func (s *fakeWatchStream) Context() context.Context     { return s.ctx }
func (s *fakeWatchStream) SendMsg(any) error            { return errors.New("not implemented") }
func (s *fakeWatchStream) RecvMsg(any) error            { return errors.New("not implemented") }

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestConnectReturnsOnStreamCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeBotStream{ctx: ctx, recv: make(chan *pb.BotIntent, 1), sent: make(chan *pb.WorldState, 10)}
	stream.recv <- &pb.BotIntent{}

	server := NewSimulationServer(services.NewSimulationEngine(800, 600, nil), 16)
	done := make(chan error, 1)
	go func() { done <- server.Connect(stream) }()

	waitForCondition(t, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		return len(server.botChannels) == 1
	})
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Connect did not return when the stream context was canceled")
	}
	server.mu.Lock()
	remaining := len(server.botChannels)
	server.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected bot channel to be removed, got %d", remaining)
	}
	// Broadcasting after disconnect must not panic on a closed channel.
	server.BroadcastState(&pb.WorldState{})
}

func TestWatchMatchReturnsOnStreamCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeWatchStream{ctx: ctx, sent: make(chan *pb.WorldState, 10)}
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, nil), 16)

	done := make(chan error, 1)
	go func() { done <- server.WatchMatch(&pb.MatchRequest{MatchId: "m"}, stream) }()

	waitForCondition(t, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		return len(server.dashboardChannels) == 1
	})
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WatchMatch did not return when the stream context was canceled")
	}
	server.mu.Lock()
	remaining := len(server.dashboardChannels)
	server.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected dashboard channel to be removed, got %d", remaining)
	}
	server.BroadcastState(&pb.WorldState{})
}

func TestStopSimulationStopsGameLoop(t *testing.T) {
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, nil), time.Millisecond)
	if _, err := server.StartSimulation(context.Background(), &pb.ArenaConfig{}); err != nil {
		t.Fatalf("StartSimulation failed: %v", err)
	}
	server.mu.Lock()
	loop := server.loop
	server.mu.Unlock()
	if loop == nil {
		t.Fatal("expected a tracked game loop")
	}

	if _, err := server.StopSimulation(context.Background(), &pb.StopSimulationRequest{}); err != nil {
		t.Fatalf("StopSimulation failed: %v", err)
	}
	select {
	case <-loop.StopChan:
	case <-time.After(time.Second):
		t.Fatal("StopSimulation did not stop the game loop")
	}
}

func TestNewWebServerTimeouts(t *testing.T) {
	srv := newWebServer(http.NewServeMux())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatalf("web server is missing timeouts: %+v", srv)
	}
	if srv.Handler == nil {
		t.Fatal("expected an explicit handler instead of DefaultServeMux")
	}
}
