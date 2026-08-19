package domain

import (
	"github.com/shopspring/decimal"
)

// SubmissionStatus models the lifecycle of a certification submission.
type SubmissionStatus string

const (
	SubmissionDraft      SubmissionStatus = "draft"
	SubmissionPending    SubmissionStatus = "pending"
	SubmissionDispatched SubmissionStatus = "dispatched"
	SubmissionTesting    SubmissionStatus = "testing"
	SubmissionReview     SubmissionStatus = "review"
	SubmissionCompleted  SubmissionStatus = "completed"
	SubmissionRejected   SubmissionStatus = "rejected"
	SubmissionCancelled  SubmissionStatus = "cancelled"
	SubmissionFrozen     SubmissionStatus = "frozen"
)

// Submission represents an enterprise's certification application for
// a specific device model under a set of applicable standards.
type Submission struct {
	ID             string           `json:"id"`
	EnterpriseID   string           `json:"enterprise_id"`
	EnterpriseName string           `json:"enterprise_name"`
	ModelNo        string           `json:"model_no"`
	ModelName      string           `json:"model_name"`
	Category       DeviceCategory   `json:"category"`
	RiskLevel      RiskLevel        `json:"risk_level"`
	StandardCodes  []string         `json:"standard_codes"`
	BatchNo        string           `json:"batch_no"`
	Status         SubmissionStatus `json:"status"`
	Charge         ChargeInfo       `json:"charge"`
	BaseCharge     decimal.Decimal  `json:"base_charge"`
	Notes          string           `json:"notes,omitempty"`
	Audit          AuditMeta        `json:"audit"`
	// IdempotencyKey is the business key for duplicate detection:
	// same model_no + batch_no => idempotent replay.
	IdempotencyKey string `json:"idempotency_key"`
	// DispatchRound tracks how many dispatch rounds have been attempted.
	DispatchRound int `json:"dispatch_round"`
}

// SubmissionStateMachine enforces valid transitions. Only the listed
// transitions are allowed; all others are rejected with ErrInvalidTransition.
var submissionTransitions = map[SubmissionStatus][]SubmissionStatus{
	SubmissionDraft:      {SubmissionPending, SubmissionCancelled},
	SubmissionPending:    {SubmissionDispatched, SubmissionCancelled, SubmissionFrozen},
	SubmissionDispatched: {SubmissionTesting, SubmissionCancelled, SubmissionFrozen},
	SubmissionTesting:    {SubmissionReview, SubmissionCompleted, SubmissionFrozen, SubmissionCancelled},
	SubmissionReview:     {SubmissionCompleted, SubmissionFrozen, SubmissionRejected},
	SubmissionFrozen:     {SubmissionReview, SubmissionCancelled},
	SubmissionCompleted:  {},
	SubmissionRejected:   {SubmissionPending},
	SubmissionCancelled:  {SubmissionPending},
}

func (s SubmissionStatus) CanTransitionTo(target SubmissionStatus) bool {
	allowed, ok := submissionTransitions[s]
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

func (s SubmissionStatus) IsTerminal() bool {
	allowed, ok := submissionTransitions[s]
	if !ok {
		return false
	}
	return len(allowed) == 0
}

// RecordDispatchRound increments the dispatch round counter.
func (sub *Submission) RecordDispatchRound() {
	sub.DispatchRound++
}

// TotalCharge returns the effective charge amount, zeroing duplicates.
func (sub *Submission) TotalCharge() decimal.Decimal {
	if sub.Charge.IsDuplicate {
		return decimal.Zero
	}
	return sub.Charge.Amount
}
