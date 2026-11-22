package routes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type routeRuntimeClient struct {
	mu     sync.Mutex
	starts []*pb.StartBotRequest
	stops  []*pb.StopBotRequest
	fail   bool
	closed bool
}

func (f *routeRuntimeClient) StartBot(_ context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, req)
	if f.fail {
		return nil, context.DeadlineExceeded
	}
	return &pb.StartBotResponse{Success: true, ContainerId: "container-" + req.BotId}, nil
}

func (f *routeRuntimeClient) StopBot(_ context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, req)
	return &pb.StopBotResponse{Success: true}, nil
}

func (f *routeRuntimeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *routeRuntimeClient) snapshot() ([]*pb.StartBotRequest, []*pb.StopBotRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.StartBotRequest(nil), f.starts...), append([]*pb.StopBotRequest(nil), f.stops...)
}

func newTestDB(t *testing.T) *persistence.Database {
	t.Helper()
	db, err := persistence.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCreateMatchValidatesAndPersists(t *testing.T) {
	db := newTestDB(t)
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, db), time.Millisecond)
	server.engine.SetBot("stale", &pb.BotState{Id: "stale", Name: "stale"})

	if _, err := server.CreateMatch(context.Background(), nil); grpcstatus.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil config code = %v, want InvalidArgument", grpcstatus.Code(err))
	}
	if _, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{Width: -1, Height: 10}); grpcstatus.Code(err) != codes.InvalidArgument {
		t.Fatalf("negative width code = %v, want InvalidArgument", grpcstatus.Code(err))
	}

	resp, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{
		Id: "match-1", Width: 1024, Height: 768,
		Zones: []*pb.Zone{{Position: &pb.Vector3{X: 5, Y: 6}, Radius: 7}},
	})
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	if resp.MatchId != "match-1" || resp.Status != pb.MatchStatus_WAITING {
		t.Errorf("unexpected response: %+v", resp)
	}
	if server.engine.GetMatchID() != "match-1" || server.engine.GetBot("stale") != nil {
		t.Errorf("engine state was not reset: match=%q bot=%v", server.engine.GetMatchID(), server.engine.GetBot("stale"))
	}

	matches, err := db.ListMatches()
	if err != nil || len(matches) != 1 {
		t.Fatalf("ListMatches = %v, %v; want one match", matches, err)
	}
	if matches[0].Status != waitingStatus || matches[0].ArenaWidth != 1024 {
		t.Errorf("unexpected persisted match: %+v", matches[0])
	}
}

func TestCreateMatchRejectsRunningSimulation(t *testing.T) {
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, nil), time.Millisecond)
	server.engine.SetStatus(pb.MatchStatus_RUNNING)

	if _, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{Width: 800, Height: 600}); grpcstatus.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CreateMatch code = %v, want FailedPrecondition", grpcstatus.Code(err))
	}
}

func TestRegisterBotStartsRuntimeManagedBot(t *testing.T) {
	db := newTestDB(t)
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, db), time.Millisecond)
	client := &routeRuntimeClient{}
	server.engine.SetRuntimeClient(client)
	server.SetBotRuntimeConfig(BotRuntimeConfig{
		Image: "bot:latest",
		ImagesByLanguage: map[string]string{
			"python": "bot-python:latest",
		},
		GameServerURL: "engine:50051",
	})

	if _, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{Id: "match-1", Width: 800, Height: 600}); err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	resp, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{
		UserId: "user-1", Name: "Python Bot", Version: "v1", Language: "PYTHON", SourceCode: "print('hi')",
	})
	if err != nil {
		t.Fatalf("RegisterBot: %v", err)
	}
	if !resp.Success || resp.BotId == "" || resp.VersionId == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if got := server.engine.GetBot(resp.BotId); got == nil || got.Name != "Python Bot" {
		t.Errorf("bot state not registered: %+v", got)
	}

	starts, _ := client.snapshot()
	if len(starts) != 1 {
		t.Fatalf("StartBot calls = %d, want 1", len(starts))
	}
	if starts[0].Image != "bot-python:latest" || starts[0].MatchId != "match-1" || starts[0].GameServerUrl != "engine:50051" {
		t.Errorf("unexpected runtime request: %+v", starts[0])
	}
}

func TestRegisterBotDirectConnectAndValidation(t *testing.T) {
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, nil), time.Millisecond)
	if _, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{UserId: "u", Name: "b"}); grpcstatus.Code(err) != codes.FailedPrecondition {
		t.Fatalf("RegisterBot without a match code = %v, want FailedPrecondition", grpcstatus.Code(err))
	}

	if _, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{Id: "match-1", Width: 800, Height: 600}); err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	if _, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{UserId: "u"}); grpcstatus.Code(err) != codes.InvalidArgument {
		t.Fatalf("RegisterBot without a name code = %v, want InvalidArgument", grpcstatus.Code(err))
	}
	resp, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{UserId: "u", Name: "direct"})
	if err != nil {
		t.Fatalf("RegisterBot direct: %v", err)
	}
	if server.engine.GetBot(resp.BotId) == nil {
		t.Error("direct-connect bot was not added to the engine")
	}

	server.engine.SetStatus(pb.MatchStatus_RUNNING)
	if _, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{UserId: "u", Name: "late"}); grpcstatus.Code(err) != codes.FailedPrecondition {
		t.Fatalf("RegisterBot while running code = %v, want FailedPrecondition", grpcstatus.Code(err))
	}
}

func TestRegisterBotReportsRuntimeFailure(t *testing.T) {
	db := newTestDB(t)
	server := NewSimulationServer(services.NewSimulationEngine(800, 600, db), time.Millisecond)
	client := &routeRuntimeClient{fail: true}
	server.engine.SetRuntimeClient(client)
	server.SetBotRuntimeConfig(BotRuntimeConfig{Image: "bot:latest"})

	if _, err := server.CreateMatch(context.Background(), &pb.ArenaConfig{Id: "match-1", Width: 800, Height: 600}); err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	if _, err := server.RegisterBot(context.Background(), &pb.RegisterBotRequest{UserId: "u", Name: "b"}); grpcstatus.Code(err) != codes.Internal {
		t.Fatalf("RegisterBot code = %v, want Internal", grpcstatus.Code(err))
	}
	if got := server.engine.GetAliveCount(); got != 0 {
		t.Errorf("alive bots after runtime failure = %d, want 0", got)
	}
}
