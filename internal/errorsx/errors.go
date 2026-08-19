package errorsx

import (
	"errors"
	"fmt"
)

// Sentinel errors used across packages. Wrap with %w at boundaries so
// callers can use errors.Is / errors.As to classify failures.
var (
	ErrNotFound          = errors.New("resource not found")
	ErrAlreadyExists     = errors.New("resource already exists")
	ErrConflict          = errors.New("state conflict")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrValidation        = errors.New("validation failed")
	ErrIdempotentReplay  = errors.New("idempotent replay")
	ErrCircuitOpen       = errors.New("circuit breaker open")
	ErrUpstreamTimeout   = errors.New("upstream timeout")
	ErrNoUpstream        = errors.New("no upstream available")
	ErrFrozenForReview   = errors.New("model frozen for review")
	ErrRuleNotActive     = errors.New("rule not active")
	ErrRuleTrialFailed   = errors.New("rule trial calculation failed")
	ErrDeadLetter        = errors.New("permanent failure recorded")
	ErrConsistencyBroken = errors.New("consistency check failed")
	ErrIncompleteTx      = errors.New("incomplete transaction")
	ErrForbidden         = errors.New("action forbidden")
	ErrQuotaExceeded     = errors.New("quota exceeded")
	ErrSLABreached       = errors.New("sla deadline breached")
)

func Wrap(err error, format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, err)...)
}

func Is(target error, errs ...error) bool {
	for _, e := range errs {
		if errors.Is(target, e) {
			return true
		}
	}
	return false
}

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("field %s: %s", e.Field, e.Message)
}

type MultiError struct {
	Fields []FieldError
}

func (m *MultiError) Add(field, msg string) {
	m.Fields = append(m.Fields, FieldError{Field: field, Message: msg})
}

func (m *MultiError) HasErrors() bool { return len(m.Fields) > 0 }

func (m *MultiError) Error() string {
	if len(m.Fields) == 0 {
		return "no errors"
	}
	s := fmt.Sprintf("%d validation error(s)", len(m.Fields))
	for _, f := range m.Fields {
		s += fmt.Sprintf("; %s: %s", f.Field, f.Message)
	}
	return s
}
