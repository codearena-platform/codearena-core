package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

// recordingRunner is safe for the concurrent start paths exercised by
// shutdown. It records the exact container IDs handed to StopContainer.
type recordingRunner struct {
	mu             sync.Mutex
	started        map[string]string // botID -> containerID
	stopped        []string
	enteredStart   chan string
	releaseStart   chan struct{}
	blockNextStart bool
}

func newRecordingRunner() *recordingRunner {
	return &recordingRunner{
		started:      map[string]string{},
		enteredStart: make(chan string, 16),
		releaseStart: make(chan struct{}),
	}
}

func (r *recordingRunner) StartContainer(_ context.Context, _ string, botID string, _ []string) (string, error) {
	if r.blockNextStart {
		r.blockNextStart = false
		r.enteredStart <- botID
		<-r.releaseStart
	}
	cid := "container-" + botID
	r.mu.Lock()
	r.started[botID] = cid
	r.mu.Unlock()
	return cid, nil
}

func (r *recordingRunner) StopContainer(_ context.Context, containerID string) error {
	r.mu.Lock()
	r.stopped = append(r.stopped, containerID)
	r.mu.Unlock()
	return nil
}

func (r *recordingRunner) CountActiveContainers(context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.started), nil
}

func (r *recordingRunner) stoppedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stopped...)
}

func TestSchedulerShutdownStopsAllManagedContainers(t *testing.T) {
	runner := newRecordingRunner()
	scheduler := NewScheduler(runner, 5)

	for _, botID := range []string{"bot-1", "bot-2"} {
		if _, _, _, err := scheduler.StartBot(context.Background(), botID, "image", "match", nil); err != nil {
			t.Fatalf("StartBot(%s): %v", botID, err)
		}
	}

	scheduler.Shutdown(context.Background())

	stopped := runner.stoppedIDs()
	if len(stopped) != 2 {
		t.Fatalf("stopped containers = %v, want bot-1 and bot-2", stopped)
	}
	if got := scheduler.GetActiveCount(); got != 0 {
		t.Errorf("active count after shutdown = %d, want 0", got)
	}
	if _, _, _, err := scheduler.StartBot(context.Background(), "bot-3", "image", "match", nil); err == nil {
		t.Error("StartBot succeeded after Shutdown")
	}

	// A second Shutdown must be a no-op, not a double stop.
	scheduler.Shutdown(context.Background())
	if got := len(runner.stoppedIDs()); got != 2 {
		t.Errorf("stopped containers after repeated shutdown = %v, want 2", got)
	}
}

func TestSchedulerShutdownDiscardsQueuedBots(t *testing.T) {
	runner := newRecordingRunner()
	scheduler := NewScheduler(runner, 1)

	if _, _, _, err := scheduler.StartBot(context.Background(), "bot-1", "image", "match", nil); err != nil {
		t.Fatalf("StartBot(bot-1): %v", err)
	}
	if _, queued, _, err := scheduler.StartBot(context.Background(), "bot-2", "image", "match", nil); err != nil || !queued {
		t.Fatalf("StartBot(bot-2): queued=%v err=%v", queued, err)
	}

	scheduler.Shutdown(context.Background())
	time.Sleep(50 * time.Millisecond) // let any queued goroutine run, if one was started

	stopped := runner.stoppedIDs()
	if len(stopped) != 1 || stopped[0] != "container-bot-1" {
		t.Errorf("stopped containers = %v, want only container-bot-1", stopped)
	}
	runner.mu.Lock()
	_, queuedStarted := runner.started["bot-2"]
	runner.mu.Unlock()
	if queuedStarted {
		t.Error("queued bot was started after Shutdown")
	}
	if got := scheduler.GetQueueSize(); got != 0 {
		t.Errorf("queue size after shutdown = %d, want 0", got)
	}
}

func TestSchedulerShutdownWaitsForInFlightStart(t *testing.T) {
	runner := newRecordingRunner()
	runner.blockNextStart = true
	scheduler := NewScheduler(runner, 2)

	startDone := make(chan error, 1)
	go func() {
		_, _, _, err := scheduler.StartBot(context.Background(), "bot-1", "image", "match", nil)
		startDone <- err
	}()

	select {
	case botID := <-runner.enteredStart:
		if botID != "bot-1" {
			t.Fatalf("start entered for %q, want bot-1", botID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartContainer was never called")
	}

	shutdownDone := make(chan struct{})
	go func() {
		scheduler.Shutdown(context.Background())
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned while a container start was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(runner.releaseStart)

	select {
	case err := <-startDone:
		if !errors.Is(err, ErrRuntimeShuttingDown) {
			t.Errorf("in-flight StartBot error = %v, want shutdown error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight StartBot did not return")
	}
	select {
	case <-shutdownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after the in-flight start finished")
	}

	if stopped := runner.stoppedIDs(); len(stopped) != 1 || stopped[0] != "container-bot-1" {
		t.Errorf("stopped containers = %v, want container-bot-1", stopped)
	}
}

func TestRuntimeServiceShutdownStopsManagedContainers(t *testing.T) {
	runner := newRecordingRunner()
	service := NewRuntimeServiceWithRunner(runner, 5)

	if _, err := service.StartBot(context.Background(), &pb.StartBotRequest{BotId: "bot-1", Image: "image"}); err != nil {
		t.Fatalf("StartBot: %v", err)
	}
	service.Shutdown(context.Background())

	if got := runner.stoppedIDs(); len(got) != 1 || got[0] != "container-bot-1" {
		t.Errorf("stopped containers = %v, want container-bot-1", got)
	}
}
