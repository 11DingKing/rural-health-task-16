package domain

import "time"

// TestResult represents the detailed conclusion from a testing agency
// for one dispatch task (one standard test).
type TestResult struct {
	ID           string    `json:"id"`
	DispatchID   string    `json:"dispatch_id"`
	SubmissionID string    `json:"submission_id"`
	ModelNo      string    `json:"model_no"`
	StandardCode string    `json:"standard_code"`
	AgencyID     string    `json:"agency_id"`
	Passed       bool      `json:"passed"`
	Score        float64   `json:"score"`
	Conclusion   string    `json:"conclusion"`
	Detail       string    `json:"detail,omitempty"`
	TestDate     time.Time `json:"test_date"`
	ReceivedAt   time.Time `json:"received_at"`
	Audit        AuditMeta `json:"audit"`
}

// SummaryStatus models the aggregate conclusion for a model.
type SummaryStatus string

const (
	SummaryPending  SummaryStatus = "pending"
	SummaryComputed SummaryStatus = "computed"
	SummaryStale    SummaryStatus = "stale"
	SummaryFrozen   SummaryStatus = "frozen"
	SummaryFinal    SummaryStatus = "final"
)

// Summary represents the aggregate conclusion for a model across all
// standard test results. It is a derived view that must stay consistent
// with the detail results.
type Summary struct {
	ID            string        `json:"id"`
	ModelNo       string        `json:"model_no"`
	BatchNo       string        `json:"batch_no"`
	SubmissionID  string        `json:"submission_id"`
	OverallPassed bool          `json:"overall_passed"`
	StandardCount int           `json:"standard_count"`
	PassedCount   int           `json:"passed_count"`
	FailedCount   int           `json:"failed_count"`
	Status        SummaryStatus `json:"status"`
	ComputedAt    time.Time     `json:"computed_at"`
	Version       int           `json:"version"`
	FrozenReason  string        `json:"frozen_reason,omitempty"`
	Audit         AuditMeta     `json:"audit"`
}

var summaryTransitions = map[SummaryStatus][]SummaryStatus{
	SummaryPending:  {SummaryComputed, SummaryFrozen},
	SummaryComputed: {SummaryStale, SummaryFinal, SummaryFrozen},
	SummaryStale:    {SummaryComputed, SummaryFrozen},
	SummaryFrozen:   {SummaryPending},
	SummaryFinal:    {},
}

func (s SummaryStatus) CanTransitionTo(target SummaryStatus) bool {
	allowed, ok := summaryTransitions[s]
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
