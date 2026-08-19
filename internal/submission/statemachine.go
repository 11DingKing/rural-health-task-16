package submission

import (
	"fmt"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
)

// StateMachine enforces valid submission status transitions.
type StateMachine struct{}

func NewStateMachine() *StateMachine { return &StateMachine{} }

// Transition validates and applies a status change. Returns
// ErrInvalidTransition for disallowed transitions.
func (sm *StateMachine) Transition(current, target domain.SubmissionStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

// DispatchStateMachine enforces dispatch task transitions.
type DispatchStateMachine struct{}

func NewDispatchStateMachine() *DispatchStateMachine { return &DispatchStateMachine{} }

func (dsm *DispatchStateMachine) Transition(current, target domain.DispatchStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

// CallbackStateMachine enforces callback transitions.
type CallbackStateMachine struct{}

func NewCallbackStateMachine() *CallbackStateMachine { return &CallbackStateMachine{} }

func (csm *CallbackStateMachine) Transition(current, target domain.CallbackStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

// RuleStateMachine enforces rule status transitions.
type RuleStateMachine struct{}

func NewRuleStateMachine() *RuleStateMachine { return &RuleStateMachine{} }

func (rsm *RuleStateMachine) Transition(current, target domain.RuleStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func (rsm *RuleStateMachine) TrialTransition(current, target domain.TrialStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

// SummaryStateMachine enforces summary transitions.
type SummaryStateMachine struct{}

func NewSummaryStateMachine() *SummaryStateMachine { return &SummaryStateMachine{} }

func (ssm *SummaryStateMachine) Transition(current, target domain.SummaryStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}
