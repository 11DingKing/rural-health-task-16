package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ruralhealth/internal/config"
)

// Manager coordinates multiple upstreams with circuit breaking, timeout,
// and failover. It selects the best available upstream and falls back
// when one is unavailable or errors.
type Manager struct {
	mu          sync.RWMutex
	upstreams   []*Upstream
	breakers    map[string]*CircuitBreaker
	traceStore  *TraceStore
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	defaultTTL  time.Duration
	now         func() time.Time
}

func NewManager(cfg config.GatewayConfig, upstreams []config.UpstreamConfig, now func() time.Time) *Manager {
	m := &Manager{
		breakers:    make(map[string]*CircuitBreaker),
		traceStore:  NewTraceStore(2000),
		maxRetries:  cfg.MaxRetries,
		baseBackoff: cfg.BaseBackoff,
		maxBackoff:  cfg.MaxBackoff,
		defaultTTL:  cfg.DefaultTimeout,
		now:         now,
	}
	for _, u := range upstreams {
		up := NewUpstream(u)
		m.upstreams = append(m.upstreams, up)
		m.breakers[up.ID] = NewCircuitBreaker(cfg.CircuitMaxFail, cfg.CircuitResetAfter, now)
	}
	return m
}

// CallResult holds the outcome of a gateway call attempt.
type CallResult struct {
	UpstreamID   string
	StatusCode   int
	ResponseBody []byte
	TraceID      string
	Error        error
	UsedFailover bool
}

// CallWithFailover tries the best upstream and falls back to alternatives
// on failure. It respects circuit breaker state and records traces.
func (m *Manager) CallWithFailover(ctx context.Context, method, path string, body []byte, dispatchID string) (CallResult, error) {
	candidates := m.availableUpstreams()
	if len(candidates) == 0 {
		return CallResult{}, errors.New("no upstream available")
	}

	var lastErr error
	for attempt, up := range candidates {
		result, err := m.callOne(ctx, up, method, path, body, dispatchID)
		if err == nil && result.StatusCode < 500 {
			result.UsedFailover = attempt > 0
			return result, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("upstream %s returned status %d", up.ID, result.StatusCode)
		}
		m.breakers[up.ID].RecordFailure()
	}

	return CallResult{}, fmt.Errorf("all upstreams failed: %w", lastErr)
}

func (m *Manager) callOne(ctx context.Context, up *Upstream, method, path string, body []byte, dispatchID string) (CallResult, error) {
	cb := m.breakers[up.ID]
	if err := cb.Allow(); err != nil {
		return CallResult{UpstreamID: up.ID, Error: err}, err
	}

	start := m.now()
	status, respBody, err := up.Call(ctx, method, path, body)
	duration := m.now().Sub(start)

	trace := TraceRecord{
		UpstreamID:   up.ID,
		DispatchID:   dispatchID,
		Method:       method,
		Path:         path,
		RequestBody:  string(body),
		ResponseBody: string(respBody),
		StatusCode:   status,
		Duration:     duration.String(),
		OccurredAt:   start,
	}
	if err != nil {
		trace.Error = err.Error()
	}
	m.traceStore.Record(trace)

	if err != nil {
		cb.RecordFailure()
		return CallResult{UpstreamID: up.ID, Error: err, TraceID: trace.ID}, err
	}
	if status >= 500 {
		cb.RecordFailure()
		return CallResult{UpstreamID: up.ID, StatusCode: status, ResponseBody: respBody, TraceID: trace.ID}, fmt.Errorf("upstream %s returned %d", up.ID, status)
	}

	cb.RecordSuccess()
	return CallResult{UpstreamID: up.ID, StatusCode: status, ResponseBody: respBody, TraceID: trace.ID}, nil
}

// availableUpstreams returns upstreams with closed or half-open circuits,
// ordered by priority.
func (m *Manager) availableUpstreams() []*Upstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Upstream
	for _, up := range m.upstreams {
		cb := m.breakers[up.ID]
		if cb.State() != CircuitOpen {
			result = append(result, up)
		}
	}
	return result
}

// UpstreamStatus reports the health of each upstream.
type UpstreamStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Failures int    `json:"failures"`
	BaseURL  string `json:"base_url"`
	Priority int    `json:"priority"`
}

func (m *Manager) Status() []UpstreamStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []UpstreamStatus
	for _, up := range m.upstreams {
		cb := m.breakers[up.ID]
		result = append(result, UpstreamStatus{
			ID:       up.ID,
			Name:     up.Name,
			State:    cb.State().String(),
			Failures: cb.Failures(),
			BaseURL:  up.BaseURL,
			Priority: up.Priority,
		})
	}
	return result
}

func (m *Manager) Traces() *TraceStore {
	return m.traceStore
}

func (m *Manager) Upstreams() []*Upstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.upstreams
}

// ResetCircuit manually resets a circuit breaker (for management API).
func (m *Manager) ResetCircuit(upstreamID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cb, ok := m.breakers[upstreamID]
	if !ok {
		return errors.New("upstream not found")
	}
	cb.mu.Lock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.mu.Unlock()
	return nil
}
