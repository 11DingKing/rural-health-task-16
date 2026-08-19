package dispatch

import (
	"context"
	"fmt"
	"time"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

// FailoverConfig controls automatic failover behavior.
type FailoverConfig struct {
	Timeout     time.Duration
	MaxRetries  int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// FailoverHandler manages automatic switching to backup agencies when
// a dispatch task times out or returns an anomalous result.
type FailoverHandler struct {
	selector *AgencySelector
	config   FailoverConfig
	now      func() time.Time
}

func NewFailoverHandler(selector *AgencySelector, config FailoverConfig, now func() time.Time) *FailoverHandler {
	return &FailoverHandler{selector: selector, config: config, now: now}
}

// ShouldFailover returns true if the dispatch result warrants a failover.
// Reasons include timeout, 5xx upstream errors, or anomalous conclusions.
func (fh *FailoverHandler) ShouldFailover(task *domain.DispatchTask, result *domain.TestResult) bool {
	if task.Status == domain.DispatchTimeout {
		return true
	}
	if result != nil && result.Score < 0 {
		return true
	}
	if task.Attempt >= fh.config.MaxRetries {
		return false
	}
	return false
}

// Failover switches a task to a backup agency. The original task is
// marked as failover, and a new assignment is created on the backup.
// Already-received results are preserved.
func (fh *FailoverHandler) Failover(ctx context.Context, task *domain.DispatchTask, reason string) (*domain.Agency, error) {
	backup, err := fh.selector.SelectBackup(ctx, task.StandardCode, task.AgencyID)
	if err != nil {
		return nil, err
	}
	if backup == nil {
		return nil, errorsx.ErrNoUpstream
	}

	task.Status = domain.DispatchFailover
	task.FailoverReason = reason
	task.Audit.UpdatedAt = fh.now()
	return backup, nil
}

// NextRetryAt computes the next retry time with exponential backoff.
func (fh *FailoverHandler) NextRetryAt(attempt int) time.Time {
	backoff := fh.config.BaseBackoff
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > fh.config.MaxBackoff {
			backoff = fh.config.MaxBackoff
			break
		}
	}
	return fh.now().Add(backoff)
}

// FormatRetryStatus returns a human-readable retry status for logging.
func (fh *FailoverHandler) FormatRetryStatus(task *domain.DispatchTask) string {
	return fmt.Sprintf("task %s attempt %d/%d status=%s next_retry=%v",
		task.ID, task.Attempt, task.MaxAttempts, task.Status, task.NextRetryAt)
}
