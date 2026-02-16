package routes

import (
	"context"
	"fmt"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *SimulationServer) StartSimulation(ctx context.Context, cfg *pb.ArenaConfig) (*pb.SimulationResponse, error) {
	loop := services.NewGameLoop(s.engine, s.TickRate, s.BroadcastState)
	loop.Start()

	return &pb.SimulationResponse{Status: pb.MatchStatus_RUNNING}, nil
}

func (s *SimulationServer) StopSimulation(ctx context.Context, req *pb.StopSimulationRequest) (*pb.SimulationResponse, error) {
	return &pb.SimulationResponse{Status: pb.MatchStatus_FINISHED}, nil
}

func (s *SimulationServer) WatchMatch(req *pb.MatchRequest, stream pb.MatchService_WatchMatchServer) error {
	out := make(chan *pb.WorldState, 10)
	s.mu.Lock()
	s.dashboardChannels[out] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.dashboardChannels, out)
		s.mu.Unlock()
	}()

	for st := range out {
		if err := stream.Send(st); err != nil {
			return err
		}
	}
	return nil
}

func (s *SimulationServer) ListActiveMatches(ctx context.Context, req *pb.Empty) (*pb.MatchList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := &pb.MatchList{}
	if s.engine.Status == pb.MatchStatus_RUNNING {
		list.Matches = append(list.Matches, &pb.MatchResponse{
			MatchId: s.engine.MatchID,
			Status:  pb.MatchStatus_RUNNING,
		})
	}
	return list, nil
}

func (s *SimulationServer) ListMatches(ctx context.Context, req *pb.Empty) (*pb.MatchList, error) {
	db := s.engine.DB
	if db == nil {
		return &pb.MatchList{}, nil
	}

	matches, err := db.ListMatches()
	if err != nil {
		return nil, err
	}

	list := &pb.MatchList{}
	for _, m := range matches {
		status := pb.MatchStatus_FINISHED
		if m.Status == "RUNNING" {
			status = pb.MatchStatus_RUNNING
		}
		list.Matches = append(list.Matches, &pb.MatchResponse{
			MatchId: m.ID,
			Status:  status,
		})
	}
	return list, nil
}

func (s *SimulationServer) GetMatchReplay(ctx context.Context, req *pb.ReplayRequest) (*pb.ReplayData, error) {
	db := s.engine.DB
	if db == nil {
		return nil, nil
	}

	events, err := db.GetEvents(req.MatchId, req.StartTick, req.EndTick)
	if err != nil {
		return nil, err
	}

	pbEvents := make([]*pb.SimulationEvent, 0, len(events))
	for _, e := range events {
		var pbEv pb.SimulationEvent
		if err := protojson.Unmarshal([]byte(e.Payload), &pbEv); err == nil {
			pbEvents = append(pbEvents, &pbEv)
		}
	}

	return &pb.ReplayData{
		MatchId: req.MatchId,
		Events:  pbEvents,
	}, nil
}

func (s *SimulationServer) GetMatchHighlights(ctx context.Context, req *pb.ReplayRequest) (*pb.HighlightsData, error) {
	db := s.engine.DB
	if db == nil {
		return nil, nil
	}

	events, err := db.GetHighlights(req.MatchId)
	if err != nil {
		return nil, err
	}

	moments := make([]*pb.HighlightMoment, 0, len(events))
	for _, e := range events {
		var pbEv pb.SimulationEvent
		if err := protojson.Unmarshal([]byte(e.Payload), &pbEv); err == nil {
			desc := "Key event"
			if death := pbEv.GetDeath(); death != nil {
				desc = fmt.Sprintf("Bot %s was destroyed by %s", death.BotId, death.KillerId)
			} else if finish := pbEv.GetMatchFinished(); finish != nil {
				desc = "Match finished"
			}

			moments = append(moments, &pb.HighlightMoment{
				Tick:        e.Tick,
				Type:        e.Type,
				Description: desc,
			})
		}
	}

	return &pb.HighlightsData{
		MatchId: req.MatchId,
		Moments: moments,
	}, nil
}
