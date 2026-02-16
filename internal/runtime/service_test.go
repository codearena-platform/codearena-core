package runtime

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

// MockBotRunner helps testing RuntimeService without actual Docker
type MockBotRunner struct {
	startFunc func(image string, botID string, env []string) (string, error)
	stopFunc  func(containerID string) error
	countFunc func() (int, error)
}

func (m *MockBotRunner) StartContainer(ctx context.Context, image string, botID string, env []string) (string, error) {
	return m.startFunc(image, botID, env)
}

func (m *MockBotRunner) StopContainer(ctx context.Context, containerID string) error {
	return m.stopFunc(containerID)
}

func (m *MockBotRunner) CountActiveContainers(ctx context.Context) (int, error) {
	return m.countFunc()
}

func TestRuntimeService_StartBot(t *testing.T) {
	mock := &MockBotRunner{
		startFunc: func(image string, botID string, env []string) (string, error) {
			if image == "invalid" {
				return "", fmt.Errorf("invalid image")
			}
			return "container-123", nil
		},
	}

	service := NewRuntimeServiceWithRunner(mock, 10)

	t.Run("Successful Start", func(t *testing.T) {
		req := &pb.StartBotRequest{
			BotId:         "bot-1",
			Image:         "codearena/bot-go",
			GameServerUrl: "localhost:50051",
			MatchId:       "match-abc",
		}
		resp, err := service.StartBot(context.Background(), req)
		if err != nil {
			t.Fatalf("StartBot failed: %v", err)
		}
		if !resp.Success {
			t.Errorf("expected success, got failure: %s", resp.ErrorMessage)
		}
		if resp.ContainerId != "container-123" {
			t.Errorf("expected container-123, got %s", resp.ContainerId)
		}
	})

	t.Run("Failed Start", func(t *testing.T) {
		req := &pb.StartBotRequest{
			BotId: "bot-2",
			Image: "invalid",
		}
		resp, err := service.StartBot(context.Background(), req)
		if err != nil {
			t.Fatalf("StartBot failed: %v", err)
		}
		if resp.Success {
			t.Error("expected failure for invalid image, got success")
		}
		if resp.ErrorMessage != "invalid image" {
			t.Errorf("expected 'invalid image' error, got %q", resp.ErrorMessage)
		}
	})
}
