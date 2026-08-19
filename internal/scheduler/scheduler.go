package scheduler

import (
	"context"
	"sync"
	"time"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/config"
	"ruralhealth/internal/logging"
)

// Task is a background job that runs periodically.
type Task interface {
	Name() string
	Interval() time.Duration
	Run(ctx context.Context) error
}

// Scheduler runs background tasks on a ticker. It supports graceful
// shutdown and tracks each task's last run, error count, and status.
type Scheduler struct {
	tasks   []Task
	wg      sync.WaitGroup
	mu      sync.RWMutex
	cancel  context.CancelFunc
	running bool
	logger  logging.Logger
	clk     clock.Clock
	stats   map[string]*TaskStats
}

type TaskStats struct {
	LastRunAt time.Time
	LastRunOK bool
	LastError string
	RunCount  int
	FailCount int
}

func New(logger logging.Logger, clk clock.Clock) *Scheduler {
	return &Scheduler{
		logger: logger,
		clk:    clk,
		stats:  make(map[string]*TaskStats),
	}
}

// Register adds a background task.
func (s *Scheduler) Register(task Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
	s.stats[task.Name()] = &TaskStats{}
}

// Start launches all registered tasks in goroutines.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	for _, task := range s.tasks {
		s.wg.Add(1)
		go s.runTask(ctx, task)
	}
	s.logger.Info(ctx, "scheduler started", "task_count", len(s.tasks))
}

// Stop gracefully stops all tasks, waiting up to the timeout.
func (s *Scheduler) Stop(timeout time.Duration) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (s *Scheduler) runTask(ctx context.Context, task Task) {
	defer s.wg.Done()
	interval := task.Interval()
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := s.clk.Now()
			err := task.Run(ctx)
			s.mu.Lock()
			stats := s.stats[task.Name()]
			if stats != nil {
				stats.LastRunAt = start
				stats.RunCount++
				if err != nil {
					stats.LastRunOK = false
					stats.LastError = err.Error()
					stats.FailCount++
				} else {
					stats.LastRunOK = true
					stats.LastError = ""
				}
			}
			s.mu.Unlock()
			if err != nil {
				s.logger.Error(ctx, "background task failed",
					"task", task.Name(), "error", err)
			}
		}
	}
}

// Stats returns the runtime statistics for all tasks.
func (s *Scheduler) Stats() map[string]*TaskStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*TaskStats, len(s.stats))
	for k, v := range s.stats {
		copy := *v
		result[k] = &copy
	}
	return result
}

// IsRunning returns whether the scheduler is active.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SchedulerConfig builds task intervals from the app config.
func SchedulerConfig(cfg config.SchedulerConfig) (dispatch, callback, consistency time.Duration) {
	return cfg.DispatchInterval, cfg.CallbackInterval, cfg.ConsistencyCheck
}
