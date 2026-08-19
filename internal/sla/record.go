package sla

import (
	"time"

	"ruralhealth/internal/domain"
)

// EscalationLevel models the SLA escalation lifecycle.
type EscalationLevel string

const (
	LevelOnTime    EscalationLevel = "on_time"
	LevelWarning   EscalationLevel = "warning"
	LevelBreached  EscalationLevel = "breached"
	LevelEscalated EscalationLevel = "escalated"
	LevelResolved  EscalationLevel = "resolved"
	LevelCancelled EscalationLevel = "cancelled"
)

// escalationTransitions is the explicit, authoritative transition table.
// Any transition not listed here is rejected with ErrInvalidTransition.
var escalationTransitions = map[EscalationLevel][]EscalationLevel{
	LevelOnTime:    {LevelWarning, LevelBreached, LevelResolved, LevelCancelled},
	LevelWarning:   {LevelBreached, LevelResolved, LevelCancelled},
	LevelBreached:  {LevelEscalated, LevelResolved, LevelCancelled},
	LevelEscalated: {LevelResolved, LevelCancelled},
	LevelResolved:  {},
	LevelCancelled: {},
}

// CanTransitionTo reports whether the transition is permitted by the table.
func (l EscalationLevel) CanTransitionTo(target EscalationLevel) bool {
	allowed, ok := escalationTransitions[l]
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

// IsTerminal reports whether no further transitions are possible.
func (l EscalationLevel) IsTerminal() bool {
	allowed, ok := escalationTransitions[l]
	if !ok {
		return true
	}
	return len(allowed) == 0
}

// IsBreached reports whether the level indicates the deadline has passed.
func (l EscalationLevel) IsBreached() bool {
	switch l {
	case LevelBreached, LevelEscalated:
		return true
	}
	return false
}

// SLARecord persists the SLA deadline and current escalation for a
// dispatch task. It cross-references the dispatch and its submission.
type SLARecord struct {
	ID           string           `json:"id"`
	DispatchID   string           `json:"dispatch_id"`
	SubmissionID string           `json:"submission_id,omitempty"`
	StandardCode string           `json:"standard_code"`
	AgencyID     string           `json:"agency_id,omitempty"`
	AssignedAt   time.Time        `json:"assigned_at"`
	Deadline     time.Time        `json:"deadline"`
	WarningAt    time.Time        `json:"warning_at"`
	Escalation   EscalationLevel  `json:"escalation"`
	EscalatedAt  *time.Time       `json:"escalated_at,omitempty"`
	Audit        domain.AuditMeta `json:"audit"`
}

// desiredLevel computes the time-driven target escalation. It only ever
// returns on_time, warning, or breached; higher levels (escalated and the
// terminal states) are reached only through explicit transitions and are
// never downgraded by time-based evaluation.
func desiredLevel(now, warningAt, deadline time.Time) EscalationLevel {
	switch {
	case now.Before(warningAt):
		return LevelOnTime
	case now.Before(deadline):
		return LevelWarning
	default:
		return LevelBreached
	}
}
