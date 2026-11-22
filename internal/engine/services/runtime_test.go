package services

import (
	"context"
	"sync"
	"testing"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

type fakeRuntimeClient struct {
	mu           sync.Mutex
	starts       []*pb.StartBotRequest
	stops        []*pb.StopBotRequest
	nextID       int
	queued       bool
	failStart    bool
	failResponse bool
	closed       bool
}

func (f *fakeRuntimeClient) StartBot(_ context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, req)
	if f.failStart {
		return nil, context.DeadlineExceeded
	}
	if f.failResponse {
		return &pb.StartBotResponse{Success: false, ErrorMessage: "denied"}, nil
	}
	f.nextID++
	return &pb.StartBotResponse{
		Success:     true,
		ContainerId: "container-" + req.BotId,
		Queued:      f.queued,
	}, nil
}

func (f *fakeRuntimeClient) StopBot(_ context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, req)
	return &pb.StopBotResponse{Success: true}, nil
}

func (f *fakeRuntimeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeRuntimeClient) snapshot() ([]*pb.StartBotRequest, []*pb.StopBotRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.StartBotRequest(nil), f.starts...),
		append([]*pb.StopBotRequest(nil), f.stops...), f.closed
}

func TestEngineManagedBotLifecycle(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	client := &fakeRuntimeClient{}
	engine.SetRuntimeClient(client)

	containerID, err := engine.StartManagedBot(context.Background(), "bot-1", "bot:latest", "match-1", "engine:50051")
	if err != nil {
		t.Fatalf("StartManagedBot: %v", err)
	}
	if containerID != "container-bot-1" {
		t.Errorf("container ID = %q, want container-bot-1", containerID)
	}

	// A duplicate call must not start a second container.
	if _, err := engine.StartManagedBot(context.Background(), "bot-1", "bot:latest", "match-1", "engine:50051"); err != nil {
		t.Fatalf("duplicate StartManagedBot: %v", err)
	}

	starts, _, _ := client.snapshot()
	if len(starts) != 1 {
		t.Fatalf("StartBot calls = %d, want 1", len(starts))
	}
	if starts[0].Image != "bot:latest" || starts[0].MatchId != "match-1" || starts[0].GameServerUrl != "engine:50051" {
		t.Errorf("unexpected StartBot request: %+v", starts[0])
	}

	if err := engine.StopManagedBot(context.Background(), "bot-1"); err != nil {
		t.Fatalf("StopManagedBot: %v", err)
	}
	// A repeated stop must be a no-op.
	if err := engine.StopManagedBot(context.Background(), "bot-1"); err != nil {
		t.Fatalf("repeated StopManagedBot: %v", err)
	}
	_, stops, _ := client.snapshot()
	if len(stops) != 1 || stops[0].ContainerId != "container-bot-1" || stops[0].BotId != "bot-1" {
		t.Errorf("unexpected StopBot calls: %+v", stops)
	}
}

func TestEngineStopManagedBotsAndCloseRuntime(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	client := &fakeRuntimeClient{}
	engine.SetRuntimeClient(client)

	for _, botID := range []string{"bot-1", "bot-2"} {
		if _, err := engine.StartManagedBot(context.Background(), botID, "bot:latest", "match-1", "engine:50051"); err != nil {
			t.Fatalf("StartManagedBot(%s): %v", botID, err)
		}
	}

	engine.StopManagedBots(context.Background())
	_, stops, _ := client.snapshot()
	if len(stops) != 2 {
		t.Errorf("StopBot calls = %d, want 2", len(stops))
	}

	engine.StopManagedBots(context.Background())
	_, stops, _ = client.snapshot()
	if len(stops) != 2 {
		t.Errorf("repeated StopManagedBots stopped extra containers: %+v", stops)
	}

	engine.CloseRuntime()
	_, _, closed := client.snapshot()
	if !closed {
		t.Error("runtime client was not closed")
	}
	if engine.Runtime != nil {
		t.Error("engine still holds a runtime client after CloseRuntime")
	}
}

func TestEngineStartManagedBotRequiresRuntimeForImage(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	if _, err := engine.StartManagedBot(context.Background(), "bot-1", "bot:latest", "match-1", "engine:50051"); err == nil {
		t.Fatal("StartManagedBot started a container without a runtime client")
	}

	// Direct-connect bots need no runtime.
	if _, err := engine.StartManagedBot(context.Background(), "bot-1", "", "match-1", "engine:50051"); err != nil {
		t.Fatalf("direct-connect StartManagedBot: %v", err)
	}
}

func TestEngineStartManagedBotHandlesRuntimeFailures(t *testing.T) {
	engine := NewSimulationEngine(800, 600, nil)
	client := &fakeRuntimeClient{failResponse: true}
	engine.SetRuntimeClient(client)
	if _, err := engine.StartManagedBot(context.Background(), "bot-1", "bot:latest", "match-1", "engine:50051"); err == nil {
		t.Fatal("StartManagedBot accepted a rejected runtime response")
	}

	client.failResponse = false
	client.queued = true
	containerID, err := engine.StartManagedBot(context.Background(), "bot-2", "bot:latest", "match-1", "engine:50051")
	if err != nil || containerID != "" {
		t.Fatalf("queued StartManagedBot = %q, %v; want empty ID and no error", containerID, err)
	}
}
