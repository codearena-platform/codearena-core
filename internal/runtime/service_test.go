package runtime

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestRuntimeService_StopBotOnlyStopsOwnedContainers(t *testing.T) {
	stopped := make([]string, 0)
	mock := &MockBotRunner{
		startFunc: func(image, botID string, env []string) (string, error) {
			return "container-123", nil
		},
		stopFunc: func(containerID string) error {
			stopped = append(stopped, containerID)
			return nil
		},
	}
	service := NewRuntimeServiceWithRunner(mock, 10)

	if _, err := service.StartBot(context.Background(), &pb.StartBotRequest{
		BotId: "bot-1", Image: "codearena/bot-go", GameServerUrl: "localhost:50051", MatchId: "match-1",
	}); err != nil {
		t.Fatalf("StartBot failed: %v", err)
	}

	tests := []struct {
		name        string
		botID       string
		containerID string
		wantCode    codes.Code
	}{
		{"foreign container for known bot", "bot-1", "other-container", codes.PermissionDenied},
		{"owned container for different bot", "bot-2", "container-123", codes.PermissionDenied},
		{"unknown container without bot id", "", "other-container", codes.PermissionDenied},
		{"missing container id", "bot-1", "", codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.StopBot(context.Background(), &pb.StopBotRequest{BotId: tt.botID, ContainerId: tt.containerID})
			if status.Code(err) != tt.wantCode {
				t.Fatalf("expected %v, got %v", tt.wantCode, err)
			}
			if len(stopped) != 0 {
				t.Fatalf("no container should have been stopped, got %v", stopped)
			}
		})
	}

	resp, err := service.StopBot(context.Background(), &pb.StopBotRequest{BotId: "bot-1", ContainerId: "container-123"})
	if err != nil || !resp.Success {
		t.Fatalf("owner should be able to stop its container, resp=%+v err=%v", resp, err)
	}
	if len(stopped) != 1 || stopped[0] != "container-123" {
		t.Fatalf("unexpected stopped containers: %v", stopped)
	}
}

func TestRuntimeService_StopBotResolvesOwnerFromContainerID(t *testing.T) {
	mock := &MockBotRunner{
		startFunc: func(image, botID string, env []string) (string, error) { return "container-123", nil },
		stopFunc:  func(containerID string) error { return nil },
	}
	service := NewRuntimeServiceWithRunner(mock, 10)
	if _, err := service.StartBot(context.Background(), &pb.StartBotRequest{BotId: "bot-1", Image: "codearena/bot-go"}); err != nil {
		t.Fatalf("StartBot failed: %v", err)
	}

	resp, err := service.StopBot(context.Background(), &pb.StopBotRequest{ContainerId: "container-123"})
	if err != nil || !resp.Success {
		t.Fatalf("expected stop to resolve owner, resp=%+v err=%v", resp, err)
	}
	if service.scheduler.GetActiveCount() != 0 {
		t.Error("expected scheduler slot to be released")
	}
}
