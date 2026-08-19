package gateway

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the circuit breaker state.
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	}
	return "unknown"
}

// CircuitBreaker implements a simple circuit breaker with configurable
// failure threshold, timeout, and half-open recovery. It wraps the
// concept of sony/gobreaker but is self-contained for testability.
type CircuitBreaker struct {
	mu            sync.Mutex
	state         CircuitState
	failures      int
	maxFailures   int
	resetAfter    time.Duration
	lastFailure   time.Time
	successOnHalf bool
	now           func() time.Time
}

func NewCircuitBreaker(maxFailures int, resetAfter time.Duration, now func() time.Time) *CircuitBreaker {
	return &CircuitBreaker{
		state:       CircuitClosed,
		maxFailures: maxFailures,
		resetAfter:  resetAfter,
		now:         now,
	}
}

// Allow checks if a request can be made. Returns an error if the circuit
// is open and the reset timeout hasn't elapsed.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		if cb.now().Sub(cb.lastFailure) >= cb.resetAfter {
			cb.state = CircuitHalfOpen
			return nil
		}
		return errors.New("circuit breaker open")
	case CircuitHalfOpen:
		return nil
	}
	return nil
}

// RecordSuccess closes the circuit or confirms half-open recovery.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// RecordFailure increments the failure count and opens the circuit
// if the threshold is exceeded.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = cb.now()
	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	// Check if we should transition from open to half-open
	if cb.state == CircuitOpen && cb.now().Sub(cb.lastFailure) >= cb.resetAfter {
		cb.state = CircuitHalfOpen
	}
	return cb.state
}

func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}
