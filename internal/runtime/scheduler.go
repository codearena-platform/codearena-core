package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

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

	// 1. Check if already active or being started
	if cid, ok := s.active[botID]; ok {
		s.mu.Unlock()
		return cid, false, 0, nil
	}

	// 2. Check Capacity
	if len(s.active) < s.maxBots {
		// Reserve slot immediately
		s.active[botID] = "pending_start"
		s.mu.Unlock()

		slog.Info("Capacity available, starting bot immediately", "bot_id", botID)
		cid, err := s.runner.StartContainer(ctx, image, botID, env)

		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			delete(s.active, botID)
			return "", false, 0, err
		}
		s.active[botID] = cid
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
	slog.Info("Capacity freed, checking queue", "bot_id", botID)

	// Try to start next in line
	s.processQueue()
}

func (s *Scheduler) processQueue() {
	if len(s.queue) == 0 {
		return
	}

	if len(s.active) < s.maxBots {
		next := s.queue[0]
		s.queue = s.queue[1:]

		s.active[next.ID] = "pending_start"
		slog.Info("Starting next bot from queue", "bot_id", next.ID)

		go func(bot *QueuedBot) {
			// Use a background context as the original request context may be canceled
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			cid, err := s.runner.StartContainer(ctx, bot.Image, bot.ID, bot.Env)
			s.mu.Lock()
			defer s.mu.Unlock()

			if err != nil {
				delete(s.active, bot.ID)
				slog.Error("Failed to start queued bot", "bot_id", bot.ID, "error", err)
				// If it failed, maybe we should try the next one too?
				s.processQueue()
			} else {
				s.active[bot.ID] = cid
				slog.Info("Queued bot started", "bot_id", bot.ID, "container_id", cid)
			}
		}(next)
	}
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
