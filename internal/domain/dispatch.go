package domain

import "time"

// DispatchStatus models the lifecycle of a task dispatched to a testing agency.
type DispatchStatus string

const (
	DispatchCreated    DispatchStatus = "created"
	DispatchAssigned   DispatchStatus = "assigned"
	DispatchInProgress DispatchStatus = "in_progress"
	DispatchCompleted  DispatchStatus = "completed"
	DispatchFailed     DispatchStatus = "failed"
	DispatchTimeout    DispatchStatus = "timeout"
	DispatchRetrying   DispatchStatus = "retrying"
	DispatchFailover   DispatchStatus = "failover"
	DispatchCancelled  DispatchStatus = "cancelled"
	DispatchDeadLetter DispatchStatus = "dead_letter"
)

// DispatchTask represents one standard test assigned to one agency for one submission.
type DispatchTask struct {
	ID             string         `json:"id"`
	SubmissionID   string         `json:"submission_id"`
	ModelNo        string         `json:"model_no"`
	BatchNo        string         `json:"batch_no"`
	StandardCode   string         `json:"standard_code"`
	AgencyID       string         `json:"agency_id"`
	AgencyName     string         `json:"agency_name"`
	Status         DispatchStatus `json:"status"`
	Priority       int            `json:"priority"`
	Attempt        int            `json:"attempt"`
	MaxAttempts    int            `json:"max_attempts"`
	NextRetryAt    *time.Time     `json:"next_retry_at,omitempty"`
	AssignedAt     time.Time      `json:"assigned_at"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	LastUpstreamID string         `json:"last_upstream_id,omitempty"`
	FailoverReason string         `json:"failover_reason,omitempty"`
	Audit          AuditMeta      `json:"audit"`
}

var dispatchTransitions = map[DispatchStatus][]DispatchStatus{
	DispatchCreated:    {DispatchAssigned, DispatchCancelled},
	DispatchAssigned:   {DispatchInProgress, DispatchTimeout, DispatchFailed, DispatchFailover, DispatchCancelled},
	DispatchInProgress: {DispatchCompleted, DispatchFailed, DispatchTimeout, DispatchFailover},
	DispatchFailed:     {DispatchRetrying, DispatchDeadLetter, DispatchCancelled},
	DispatchTimeout:    {DispatchRetrying, DispatchFailover, DispatchDeadLetter, DispatchCancelled},
	DispatchRetrying:   {DispatchAssigned, DispatchDeadLetter, DispatchCancelled},
	DispatchFailover:   {DispatchAssigned, DispatchDeadLetter, DispatchCancelled},
	DispatchCompleted:  {},
	DispatchDeadLetter: {DispatchAssigned},
	DispatchCancelled:  {},
}

func (d DispatchStatus) CanTransitionTo(target DispatchStatus) bool {
	allowed, ok := dispatchTransitions[d]
	if !ok {
		return false
	}
	for _, t := range allowed {
		if t == target {
			return true
		}
	}
	return false
}

func (d DispatchStatus) IsTerminal() bool {
	allowed, ok := dispatchTransitions[d]
	if !ok {
		return false
	}
	return len(allowed) == 0
}

// NeedsRetry returns true if the status warrants a retry attempt.
func (d DispatchStatus) NeedsRetry() bool {
	switch d {
	case DispatchFailed, DispatchTimeout:
		return true
	}
	return false
}

// CalculateNextRetry computes exponential backoff with jitter-free deterministic offset.
func CalculateNextRetry(base time.Time, attempt int, baseBackoff time.Duration) time.Time {
	backoff := baseBackoff
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > 30*time.Minute {
			backoff = 30 * time.Minute
			break
		}
	}
	return base.Add(backoff)
}
