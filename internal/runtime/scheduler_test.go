package runtime

import (
	"context"
	"testing"
	"time"
)

type MockRunner struct {
	startCount int
	stopCount  int
}

func (m *MockRunner) StartContainer(ctx context.Context, image string, botID string, env []string) (string, error) {
	m.startCount++
	return "container-" + botID, nil
}

func (m *MockRunner) StopContainer(ctx context.Context, containerID string) error {
	m.stopCount++
	return nil
}

func (m *MockRunner) CountActiveContainers(ctx context.Context) (int, error) {
	return m.startCount - m.stopCount, nil
}

func TestScheduler_Capacity(t *testing.T) {
	runner := &MockRunner{}
	s := NewScheduler(runner, 2)

	// Start 1
	cid, queued, pos, err := s.StartBot(context.Background(), "bot1", "image", "match", []string{})
	if err != nil || queued || cid != "container-bot1" {
		t.Fatalf("Expected immediate start for bot1, got: cid=%s, queued=%v, err=%v", cid, queued, err)
	}

	// Start 2
	cid, queued, pos, err = s.StartBot(context.Background(), "bot2", "image", "match", []string{})
	if err != nil || queued || cid != "container-bot2" {
		t.Fatalf("Expected immediate start for bot2, got: cid=%s, queued=%v, err=%v", cid, queued, err)
	}

	// Start 3 (Should be queued)
	cid, queued, pos, err = s.StartBot(context.Background(), "bot3", "image", "match", []string{})
	if err != nil || !queued || pos != 1 {
		t.Fatalf("Expected bot3 to be queued at pos 1, got: cid=%s, queued=%v, pos=%d", cid, queued, pos)
	}

	if s.GetActiveCount() != 2 {
		t.Errorf("Expected 2 active bots, got %d", s.GetActiveCount())
	}

	// Stop bot1
	s.NotifyStop("bot1")

	// Wait for scheduler to process queue
	time.Sleep(100 * time.Millisecond)

	if s.GetActiveCount() != 2 {
		t.Errorf("Expected 2 active bots after processing queue, got %d", s.GetActiveCount())
	}
	if s.GetQueueSize() != 0 {
		t.Errorf("Expected empty queue, got %d", s.GetQueueSize())
	}
}

func TestScheduler_DuplicateStart(t *testing.T) {
	runner := &MockRunner{}
	s := NewScheduler(runner, 5)

	s.StartBot(context.Background(), "bot1", "image", "match", []string{})
	s.StartBot(context.Background(), "bot1", "image", "match", []string{}) // Duplicate

	if s.GetActiveCount() != 1 {
		t.Errorf("Expected only 1 active instance for bot1, got %d", s.GetActiveCount())
	}
}
