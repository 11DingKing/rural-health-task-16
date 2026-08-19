package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubmissionStatus_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from SubmissionStatus
		to   SubmissionStatus
	}{
		{SubmissionDraft, SubmissionPending},
		{SubmissionPending, SubmissionDispatched},
		{SubmissionDispatched, SubmissionTesting},
		{SubmissionTesting, SubmissionReview},
		{SubmissionTesting, SubmissionCompleted},
		{SubmissionTesting, SubmissionFrozen},
		{SubmissionReview, SubmissionCompleted},
		{SubmissionFrozen, SubmissionReview},
		{SubmissionCancelled, SubmissionPending},
	}
	for _, tc := range validCases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be valid", tc.from, tc.to)
	}
}

func TestSubmissionStatus_InvalidTransition(t *testing.T) {
	invalidCases := []struct {
		from SubmissionStatus
		to   SubmissionStatus
	}{
		{SubmissionDraft, SubmissionCompleted},
		{SubmissionCompleted, SubmissionPending},
		{SubmissionPending, SubmissionCompleted},
		{SubmissionDispatched, SubmissionDraft},
		{SubmissionTesting, SubmissionDraft},
	}
	for _, tc := range invalidCases {
		assert.False(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be invalid", tc.from, tc.to)
	}
}

func TestSubmissionStatus_TerminalStates(t *testing.T) {
	assert.True(t, SubmissionCompleted.IsTerminal())
	assert.False(t, SubmissionCancelled.IsTerminal(), "cancelled can be reactivated")
	assert.False(t, SubmissionPending.IsTerminal())
	assert.False(t, SubmissionTesting.IsTerminal())
}

func TestDispatchStatus_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from DispatchStatus
		to   DispatchStatus
	}{
		{DispatchCreated, DispatchAssigned},
		{DispatchAssigned, DispatchInProgress},
		{DispatchInProgress, DispatchCompleted},
		{DispatchFailed, DispatchRetrying},
		{DispatchTimeout, DispatchFailover},
		{DispatchRetrying, DispatchAssigned},
		{DispatchDeadLetter, DispatchAssigned},
	}
	for _, tc := range validCases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be valid", tc.from, tc.to)
	}
}

func TestDispatchStatus_InvalidTransition(t *testing.T) {
	assert.False(t, DispatchCreated.CanTransitionTo(DispatchCompleted))
	assert.False(t, DispatchCompleted.CanTransitionTo(DispatchAssigned))
	assert.False(t, DispatchCancelled.CanTransitionTo(DispatchInProgress))
}

func TestDispatchStatus_NeedsRetry(t *testing.T) {
	assert.True(t, DispatchFailed.NeedsRetry())
	assert.True(t, DispatchTimeout.NeedsRetry())
	assert.False(t, DispatchCompleted.NeedsRetry())
	assert.False(t, DispatchAssigned.NeedsRetry())
}

func TestCalculateNextRetry_BackoffGrows(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	backoff := 100 * time.Millisecond
	t1 := CalculateNextRetry(base, 1, backoff)
	t2 := CalculateNextRetry(base, 2, backoff)
	t3 := CalculateNextRetry(base, 3, backoff)
	assert.True(t, t2.After(t1), "attempt 2 should retry later than attempt 1")
	assert.True(t, t3.After(t2), "attempt 3 should retry later than attempt 2")
}

func TestCallbackStatus_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from CallbackStatus
		to   CallbackStatus
	}{
		{CallbackPending, CallbackDelivering},
		{CallbackDelivering, CallbackDelivered},
		{CallbackDelivered, CallbackConfirmed},
		{CallbackFailed, CallbackPending},
		{CallbackDead, CallbackPending},
	}
	for _, tc := range validCases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be valid", tc.from, tc.to)
	}
}

func TestCallbackStatus_TerminalStates(t *testing.T) {
	assert.True(t, CallbackConfirmed.IsTerminal())
	assert.False(t, CallbackPending.IsTerminal())
}

func TestRuleStatus_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from RuleStatus
		to   RuleStatus
	}{
		{RuleDraft, RuleTrialing},
		{RuleTrialing, RulePending},
		{RulePending, RuleActive},
		{RuleActive, RuleSuperseded},
		{RuleSuperseded, RuleRetired},
	}
	for _, tc := range validCases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be valid", tc.from, tc.to)
	}
}

func TestRuleStatus_InvalidTransition(t *testing.T) {
	assert.False(t, RuleDraft.CanTransitionTo(RuleActive))
	assert.False(t, RuleActive.CanTransitionTo(RuleDraft))
	assert.False(t, RuleRetired.CanTransitionTo(RuleActive))
}

func TestTrialStatus_ValidTransitions(t *testing.T) {
	assert.True(t, TrialCreated.CanTransitionTo(TrialCompleted))
	assert.True(t, TrialCreated.CanTransitionTo(TrialFailed))
	assert.True(t, TrialCompleted.CanTransitionTo(TrialApproved))
	assert.True(t, TrialCompleted.CanTransitionTo(TrialRejected))
	assert.False(t, TrialApproved.CanTransitionTo(TrialRejected))
}

func TestSummaryStatus_ValidTransitions(t *testing.T) {
	validCases := []struct {
		from SummaryStatus
		to   SummaryStatus
	}{
		{SummaryPending, SummaryComputed},
		{SummaryComputed, SummaryStale},
		{SummaryComputed, SummaryFinal},
		{SummaryStale, SummaryComputed},
		{SummaryPending, SummaryFrozen},
	}
	for _, tc := range validCases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s -> %s should be valid", tc.from, tc.to)
	}
}

func TestSummaryStatus_InvalidTransition(t *testing.T) {
	assert.False(t, SummaryFinal.CanTransitionTo(SummaryPending))
	assert.False(t, SummaryFrozen.CanTransitionTo(SummaryFinal))
}

func TestAgency_CanHandleStandard(t *testing.T) {
	a := Agency{
		IsActive:            true,
		AccreditedStandards: []string{"GB-9706", "YY-0505"},
	}
	assert.True(t, a.CanHandleStandard("GB-9706"))
	assert.False(t, a.CanHandleStandard("ISO-13485"))
	a.IsActive = false
	assert.False(t, a.CanHandleStandard("GB-9706"))
}

func TestAgency_IsAvailable(t *testing.T) {
	a := Agency{IsActive: true, MaxConcurrent: 5}
	assert.True(t, a.IsAvailable(3))
	assert.False(t, a.IsAvailable(5))
	a.IsActive = false
	assert.False(t, a.IsAvailable(0))
}

func TestPaginationParams_Normalize(t *testing.T) {
	p := PaginationParams{Page: 0, PageSize: 0}
	n := p.Normalize()
	assert.Equal(t, 1, n.Page)
	assert.Equal(t, 20, n.PageSize)

	p = PaginationParams{Page: 5, PageSize: 50}
	n = p.Normalize()
	assert.Equal(t, 5, n.Page)
	assert.Equal(t, 50, n.PageSize)

	p = PaginationParams{Page: 1, PageSize: 500}
	n = p.Normalize()
	assert.Equal(t, 20, n.PageSize)
}

func TestPaginationParams_Offset(t *testing.T) {
	p := PaginationParams{Page: 3, PageSize: 20}.Normalize()
	assert.Equal(t, 40, p.Offset())
}
