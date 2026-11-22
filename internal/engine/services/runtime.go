package services

import (
	"context"
	"fmt"
	"log/slog"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

// RuntimeClient is implemented by internal/runtime.GRPCClient and by test
// doubles. The engine only needs bot start/stop plus connection cleanup.
type RuntimeClient interface {
	StartBot(ctx context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error)
	StopBot(ctx context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error)
	Close() error
}

// SetRuntimeClient installs the runtime used to sandbox bots. Passing nil
// disables runtime-managed bots; the previous client is closed if replaced.
func (e *SimulationEngine) SetRuntimeClient(client RuntimeClient) {
	e.mu.Lock()
	previous := e.Runtime
	e.Runtime = client
	e.mu.Unlock()
	if previous != nil && previous != client {
		_ = previous.Close()
	}
}

// StartManagedBot asks the runtime to start a container for a bot and records
// the container ID so it can be stopped when the match ends. A nil client is a
// no-op for deployments where bots connect to the engine directly.
func (e *SimulationEngine) StartManagedBot(ctx context.Context, botID, image, matchID, gameServerURL string) (string, error) {
	e.mu.RLock()
	client := e.Runtime
	existing := e.managedBots[botID]
	e.mu.RUnlock()

	if existing != "" {
		return existing, nil
	}
	if client == nil {
		if image != "" {
			return "", fmt.Errorf("runtime client is not configured for bot %s", botID)
		}
		return "", nil
	}
	if image == "" {
		return "", nil
	}

	resp, err := client.StartBot(ctx, &pb.StartBotRequest{
		BotId:         botID,
		Image:         image,
		MatchId:       matchID,
		GameServerUrl: gameServerURL,
	})
	if err != nil {
		return "", fmt.Errorf("start bot %s: %w", botID, err)
	}
	if resp == nil || !resp.Success {
		msg := "runtime rejected bot start"
		if resp != nil && resp.ErrorMessage != "" {
			msg = resp.ErrorMessage
		}
		return "", fmt.Errorf("start bot %s: %s", botID, msg)
	}
	if resp.Queued || resp.ContainerId == "" {
		// Queued bots are started by the runtime scheduler; this engine has no
		// container ID to stop and the runtime owns their cleanup.
		return "", nil
	}

	e.mu.Lock()
	e.managedBots[botID] = resp.ContainerId
	e.mu.Unlock()
	return resp.ContainerId, nil
}

// StopManagedBot stops a single managed bot container.
func (e *SimulationEngine) StopManagedBot(ctx context.Context, botID string) error {
	e.mu.Lock()
	client := e.Runtime
	containerID := e.managedBots[botID]
	delete(e.managedBots, botID)
	e.mu.Unlock()

	if client == nil || containerID == "" {
		return nil
	}
	resp, err := client.StopBot(ctx, &pb.StopBotRequest{BotId: botID, ContainerId: containerID})
	if err != nil {
		return fmt.Errorf("stop bot %s: %w", botID, err)
	}
	if resp != nil && !resp.Success {
		return fmt.Errorf("stop bot %s: runtime reported failure", botID)
	}
	return nil
}

// StopManagedBots stops every container created for this engine's match.
func (e *SimulationEngine) StopManagedBots(ctx context.Context) {
	e.mu.Lock()
	client := e.Runtime
	managed := e.managedBots
	e.managedBots = make(map[string]string)
	e.mu.Unlock()

	if client == nil {
		return
	}
	for botID, containerID := range managed {
		if _, err := client.StopBot(ctx, &pb.StopBotRequest{BotId: botID, ContainerId: containerID}); err != nil {
			slog.Warn("Failed to stop managed bot", "bot_id", botID, "container_id", containerID, "error", err)
		}
	}
}

// CloseRuntime releases the runtime connection. Call during engine shutdown.
func (e *SimulationEngine) CloseRuntime() {
	e.mu.Lock()
	client := e.Runtime
	e.Runtime = nil
	e.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}
