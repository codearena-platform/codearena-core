package routes

import (
	"fmt"
	"log"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func (s *SimulationServer) Connect(stream pb.BotService_ConnectServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	botID := fmt.Sprintf("bot_%d", time.Now().UnixNano())
	log.Printf("Bot %s connected", botID)

	s.mu.Lock()
	count := len(s.botChannels)
	posX, posY := float32(100), float32(100)
	if count == 1 {
		posX, posY = 600, 400
	}
	s.mu.Unlock()

	s.engine.SetBot(botID, &pb.BotState{
		Id: botID, Name: botID, Position: &pb.Vector3{X: posX, Y: posY}, Hull: 100, Energy: 150,
	})

	out := make(chan *pb.WorldState, 10)
	s.mu.Lock()
	s.botChannels[botID] = out
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.botChannels, botID)
		s.mu.Unlock()
	}()

	go func() {
		for {
			in, err := stream.Recv()
			if err != nil {
				return
			}
			s.engine.SetBotIntent(botID, in)
		}
	}()

	for st := range out {
		if err := stream.Send(st); err != nil {
			return err
		}
	}
	return nil
}

func (s *SimulationServer) BroadcastState(st *pb.WorldState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Broadcast to Bots (Filtered)
	for botID, ch := range s.botChannels {
		filteredState := s.engine.Physics.FilterStateForBot(botID, st)
		select {
		case ch <- filteredState:
		default:
		}
	}

	// 2. Broadcast to Dashboards (Full State)
	for ch := range s.dashboardChannels {
		select {
		case ch <- st:
		default:
		}
	}
}
