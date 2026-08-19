package domain

import "time"

// Standard represents an applicable certification standard (e.g. GB 9706).
type Standard struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Category    DeviceCategory `json:"category"`
	IsActive    bool           `json:"is_active"`
	Audit       AuditMeta      `json:"audit"`
}

// Rule defines conditions that determine whether a standard applies to a
// given submission context. Rules are versioned by effective date.
type Rule struct {
	ID           string     `json:"id"`
	StandardCode string     `json:"standard_code"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Expression   string     `json:"expression"`
	Priority     int        `json:"priority"`
	Status       RuleStatus `json:"status"`
	Audit        AuditMeta  `json:"audit"`
}

type RuleStatus string

const (
	RuleDraft      RuleStatus = "draft"
	RuleTrialing   RuleStatus = "trialing"
	RulePending    RuleStatus = "pending_review"
	RuleActive     RuleStatus = "active"
	RuleSuperseded RuleStatus = "superseded"
	RuleRetired    RuleStatus = "retired"
)

// RuleVersion captures a snapshot of a rule at a point in time.
// Historical conclusions are always evaluated against the rule version
// that was active when the submission was created.
type RuleVersion struct {
	ID            string     `json:"id"`
	RuleID        string     `json:"rule_id"`
	StandardCode  string     `json:"standard_code"`
	Version       int        `json:"version"`
	Expression    string     `json:"expression"`
	Priority      int        `json:"priority"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	Status        RuleStatus `json:"status"`
	Audit         AuditMeta  `json:"audit"`
}

// RuleTrial records a trial calculation performed before a rule goes live.
// The trial must show matched evidence and pass review.
type RuleTrial struct {
	ID           string          `json:"id"`
	RuleID       string          `json:"rule_id"`
	Expression   string          `json:"expression"`
	ContextJSON  string          `json:"context_json"`
	Matched      bool            `json:"matched"`
	Evidence     []TrialEvidence `json:"evidence"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Status       TrialStatus     `json:"status"`
	ReviewedBy   string          `json:"reviewed_by,omitempty"`
	ReviewedAt   *time.Time      `json:"reviewed_at,omitempty"`
	Audit        AuditMeta       `json:"audit"`
}

type TrialStatus string

const (
	TrialCreated   TrialStatus = "created"
	TrialCompleted TrialStatus = "completed"
	TrialFailed    TrialStatus = "failed"
	TrialApproved  TrialStatus = "approved"
	TrialRejected  TrialStatus = "rejected"
)

type TrialEvidence struct {
	NodeID   string `json:"node_id"`
	NodeType string `json:"node_type"`
	Source   string `json:"source"`
	GotValue string `json:"got_value"`
	Expected string `json:"expected,omitempty"`
	Passed   bool   `json:"passed"`
}

// RuleStatusTransitionTable validates transitions for rules.
var ruleTransitions = map[RuleStatus][]RuleStatus{
	RuleDraft:      {RuleTrialing, RuleRetired},
	RuleTrialing:   {RulePending, RuleDraft},
	RulePending:    {RuleActive, RuleRetired},
	RuleActive:     {RuleSuperseded, RuleRetired},
	RuleSuperseded: {RuleRetired},
	RuleRetired:    {},
}

func (r RuleStatus) CanTransitionTo(target RuleStatus) bool {
	allowed, ok := ruleTransitions[r]
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

func (t TrialStatus) CanTransitionTo(target TrialStatus) bool {
	switch t {
	case TrialCreated:
		return target == TrialCompleted || target == TrialFailed
	case TrialCompleted:
		return target == TrialApproved || target == TrialRejected
	case TrialFailed:
		return target == TrialCreated
	case TrialApproved, TrialRejected:
		return false
	}
	return false
}
