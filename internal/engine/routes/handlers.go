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
		// BroadcastState only sends while holding s.mu; deleting first means no
		// sender can race with closing the channel.
		s.mu.Lock()
		delete(s.botChannels, botID)
		close(out)
		s.mu.Unlock()
	}()

	// Recv is unblocked by gRPC when the stream context is canceled. It has its
	// own exit path so a failed Send cannot leave this goroutine behind.
	go func() {
		for {
			if stream.Context().Err() != nil {
				return
			}
			in, err := stream.Recv()
			if err != nil {
				return
			}
			s.engine.SetBotIntent(botID, in)
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case st, ok := <-out:
			if !ok {
				return nil
			}
			if err := stream.Send(st); err != nil {
				return err
			}
		}
	}
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
