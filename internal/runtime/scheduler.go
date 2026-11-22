package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrRuntimeShuttingDown is returned when a bot cannot be started because the
// scheduler is draining.
var ErrRuntimeShuttingDown = errors.New("runtime is shutting down")

type QueuedBot struct {
	ID    string
	Image string
	Env   []string
	Match string
}

type Scheduler struct {
	maxBots int
	active  map[string]string // botID -> containerID
	queue   []*QueuedBot
	runner  BotRunner
	mu      sync.Mutex
	closed  bool
	starts  sync.WaitGroup // in-flight StartContainer calls
}

func NewScheduler(runner BotRunner, maxBots int) *Scheduler {
	if maxBots <= 0 {
		maxBots = 10 // Default
	}
	return &Scheduler{
		maxBots: maxBots,
		active:  make(map[string]string),
		queue:   make([]*QueuedBot, 0),
		runner:  runner,
	}
}

func (s *Scheduler) StartBot(ctx context.Context, botID, image, matchID string, env []string) (string, bool, int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", false, 0, ErrRuntimeShuttingDown
	}

	// 1. Check if already active or being started
	if cid, ok := s.active[botID]; ok {
		s.mu.Unlock()
		return cid, false, 0, nil
	}

	// 2. Check Capacity
	if len(s.active) < s.maxBots {
		// Reserve slot immediately
		s.active[botID] = "pending_start"
		s.starts.Add(1)
		s.mu.Unlock()

		slog.Info("Capacity available, starting bot immediately", "bot_id", botID)
		cid, err := s.runner.StartContainer(ctx, image, botID, env)
		s.starts.Done()

		s.mu.Lock()
		if err != nil {
			delete(s.active, botID)
			s.mu.Unlock()
			return "", false, 0, err
		}
		if s.closed {
			// Shutdown started while the container was being created; do not
			// leave an unmanaged container behind.
			delete(s.active, botID)
			s.mu.Unlock()
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			if stopErr := s.runner.StopContainer(stopCtx, cid); stopErr != nil {
				slog.Warn("Failed to stop container created during shutdown", "container_id", cid, "error", stopErr)
			}
			return "", false, 0, ErrRuntimeShuttingDown
		}
		s.active[botID] = cid
		s.mu.Unlock()
		return cid, false, 0, nil
	}

	// 3. Enqueue
	qBot := &QueuedBot{
		ID:    botID,
		Image: image,
		Env:   env,
		Match: matchID,
	}
	s.queue = append(s.queue, qBot)
	pos := len(s.queue)
	s.mu.Unlock()

	slog.Info("Bot enqueued (capacity reached)", "bot_id", botID, "queue_pos", pos, "limit", s.maxBots)
	return "", true, pos, nil
}

func (s *Scheduler) NotifyStop(botID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.active, botID)
	if s.closed {
		return
	}
	slog.Info("Capacity freed, checking queue", "bot_id", botID)

	// Try to start next in line
	s.processQueue()
}

// processQueue must be called with s.mu held.
func (s *Scheduler) processQueue() {
	if s.closed || len(s.queue) == 0 {
		return
	}

	if len(s.active) < s.maxBots {
		next := s.queue[0]
		s.queue = s.queue[1:]

		s.active[next.ID] = "pending_start"
		s.starts.Add(1)
		slog.Info("Starting next bot from queue", "bot_id", next.ID)

		go func(bot *QueuedBot) {
			defer s.starts.Done()
			// Use a background context as the original request context may be canceled
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			cid, err := s.runner.StartContainer(ctx, bot.Image, bot.ID, bot.Env)
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				if err == nil && cid != "" {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					if stopErr := s.runner.StopContainer(stopCtx, cid); stopErr != nil {
						slog.Warn("Failed to stop queued container created during shutdown", "container_id", cid, "error", stopErr)
					}
				}
				return
			}
			if err != nil {
				delete(s.active, bot.ID)
				slog.Error("Failed to start queued bot", "bot_id", bot.ID, "error", err)
				s.processQueue()
				s.mu.Unlock()
				return
			}
			s.active[bot.ID] = cid
			s.mu.Unlock()
			slog.Info("Queued bot started", "bot_id", bot.ID, "container_id", cid)
		}(next)
	}
}

// Shutdown stops accepting bots, waits for in-flight starts, and stops every
// container this scheduler owns. It is safe to call more than once.
func (s *Scheduler) Shutdown(ctx context.Context) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.queue = nil
	containers := activeContainerIDs(s.active)
	s.active = make(map[string]string)
	s.mu.Unlock()

	// All Add calls happen under s.mu and only while !s.closed, so no new start
	// can be registered after the flag above was set.
	started := make(chan struct{})
	go func() {
		s.starts.Wait()
		close(started)
	}()
	select {
	case <-started:
	case <-ctx.Done():
	}

	// Catch containers that finished starting while we were waiting.
	s.mu.Lock()
	containers = appendUnique(containers, activeContainerIDs(s.active))
	s.active = make(map[string]string)
	s.mu.Unlock()

	// Container cleanup is best-effort but must still run if the caller's
	// context expired while we were waiting for in-flight starts.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	for _, cid := range containers {
		if err := s.runner.StopContainer(stopCtx, cid); err != nil {
			slog.Warn("Failed to stop managed bot container", "container_id", cid, "error", err)
		}
	}
}

func activeContainerIDs(active map[string]string) []string {
	ids := make([]string, 0, len(active))
	for _, cid := range active {
		if cid != "" && cid != "pending_start" {
			ids = append(ids, cid)
		}
	}
	return ids
}

func appendUnique(dst []string, values []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, value := range dst {
		seen[value] = true
	}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			dst = append(dst, value)
		}
	}
	return dst
}

// OwnedContainer reports whether containerID belongs to botID. A bot whose
// container is still starting has no container ID and owns nothing yet.
func (s *Scheduler) OwnedContainer(botID, containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.active[botID]
	return ok && id != "pending_start" && id == containerID
}

// BotForContainer looks up the bot that owns a container ID.
func (s *Scheduler) BotForContainer(containerID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for botID, id := range s.active {
		if id != "pending_start" && id == containerID {
			return botID, true
		}
	}
	return "", false
}

func (s *Scheduler) GetActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active)
}

func (s *Scheduler) GetQueueSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}
