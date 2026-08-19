package domain

import "time"

// CallbackStatus models the notification lifecycle sent back to the enterprise.
type CallbackStatus string

const (
	CallbackPending    CallbackStatus = "pending"
	CallbackDelivering CallbackStatus = "delivering"
	CallbackDelivered  CallbackStatus = "delivered"
	CallbackFailed     CallbackStatus = "failed"
	CallbackDead       CallbackStatus = "dead"
	CallbackConfirmed  CallbackStatus = "confirmed"
)

// Callback represents a notification pushed to the enterprise about their
// submission's results. Retries with backoff until confirmed or dead.
type Callback struct {
	ID           string         `json:"id"`
	SubmissionID string         `json:"submission_id"`
	EnterpriseID string         `json:"enterprise_id"`
	WebhookURL   string         `json:"webhook_url"`
	Payload      string         `json:"payload"`
	Signature    string         `json:"signature"`
	Status       CallbackStatus `json:"status"`
	Attempt      int            `json:"attempt"`
	MaxAttempts  int            `json:"max_attempts"`
	NextRetryAt  *time.Time     `json:"next_retry_at,omitempty"`
	LastError    string         `json:"last_error,omitempty"`
	DeliveredAt  *time.Time     `json:"delivered_at,omitempty"`
	ConfirmedAt  *time.Time     `json:"confirmed_at,omitempty"`
	Audit        AuditMeta      `json:"audit"`
}

var callbackTransitions = map[CallbackStatus][]CallbackStatus{
	CallbackPending:    {CallbackDelivering, CallbackFailed, CallbackDead},
	CallbackDelivering: {CallbackDelivered, CallbackFailed, CallbackDead, CallbackPending},
	CallbackDelivered:  {CallbackConfirmed, CallbackPending},
	CallbackFailed:     {CallbackPending, CallbackDead},
	CallbackDead:       {CallbackPending},
	CallbackConfirmed:  {},
}

func (c CallbackStatus) CanTransitionTo(target CallbackStatus) bool {
	allowed, ok := callbackTransitions[c]
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

func (c CallbackStatus) IsTerminal() bool {
	allowed, ok := callbackTransitions[c]
	if !ok {
		return false
	}
	return len(allowed) == 0
}

// IdempotencyRecord stores the result of an idempotent operation so
// duplicate requests return the first result without side effects.
type IdempotencyRecord struct {
	Key         string    `json:"key"`
	BusinessKey string    `json:"business_key"`
	Outcome     string    `json:"outcome"`
	ResultJSON  string    `json:"result_json"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// DeadLetter stores permanent failures for later inspection and manual retry.
type DeadLetter struct {
	ID            string     `json:"id"`
	EntityID      string     `json:"entity_id"`
	EntityType    string     `json:"entity_type"`
	Reason        string     `json:"reason"`
	LastError     string     `json:"last_error"`
	Attempts      int        `json:"attempts"`
	CreatedAt     time.Time  `json:"created_at"`
	LastAttemptAt time.Time  `json:"last_attempt_at"`
	Resolved      bool       `json:"resolved"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy    string     `json:"resolved_by,omitempty"`
}

// UpstreamTrace records the gateway's request/response for each upstream
// call, enabling full audit trails and failover tracking.
type UpstreamTrace struct {
	ID           string    `json:"id"`
	UpstreamID   string    `json:"upstream_id"`
	DispatchID   string    `json:"dispatch_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
	StatusCode   int       `json:"status_code"`
	Duration     string    `json:"duration"`
	Error        string    `json:"error,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}
