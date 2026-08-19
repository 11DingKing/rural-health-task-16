package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/config"
)

func testManager(t *testing.T, upstreams ...testUpstream) *Manager {
	cfg := config.GatewayConfig{
		DefaultTimeout:    5 * time.Second,
		MaxRetries:        3,
		BaseBackoff:       10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		CircuitMaxFail:    3,
		CircuitResetAfter: 200 * time.Millisecond,
	}
	var ups []config.UpstreamConfig
	for _, u := range upstreams {
		ups = append(ups, config.UpstreamConfig{
			ID: u.id, Name: u.name, BaseURL: u.url,
			Timeout: 2 * time.Second, Priority: u.priority,
		})
	}
	return NewManager(cfg, ups, time.Now)
}

type testUpstream struct {
	id       string
	name     string
	url      string
	priority int
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, time.Now)
	assert.Equal(t, CircuitClosed, cb.State())

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitClosed, cb.State(), "still closed at 2 failures")

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State(), "open at 3 failures")
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(2, 50*time.Millisecond, func() time.Time { return now })

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	now = now.Add(60 * time.Millisecond)
	assert.Equal(t, CircuitHalfOpen, cb.State(), "should transition to half-open after reset time")
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(1, 50*time.Millisecond, func() time.Time { return now })

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	now = now.Add(60 * time.Millisecond)
	assert.Equal(t, CircuitHalfOpen, cb.State())

	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.State(), "should close after success in half-open")
}

func TestCircuitBreaker_AllowBlocksWhenOpen(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(1, 100*time.Millisecond, func() time.Time { return now })

	cb.RecordFailure()
	err := cb.Allow()
	assert.Error(t, err, "should block when open")
}

func TestCircuitBreaker_AllowWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker(5, 100*time.Millisecond, time.Now)
	err := cb.Allow()
	assert.NoError(t, err)
}

func TestManager_CallWithFailover(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"passed": true, "score": 95, "conclusion": "PASS"})
	}))
	defer backup.Close()

	mgr := testManager(t,
		testUpstream{id: "primary", name: "Primary", url: primary.URL, priority: 1},
		testUpstream{id: "backup", name: "Backup", url: backup.URL, priority: 2},
	)

	result, err := mgr.CallWithFailover(context.Background(), "POST", "/api/v1/test", []byte("{}"), "dsp_test")
	require.NoError(t, err)
	assert.Equal(t, "backup", result.UpstreamID)
	assert.True(t, result.UsedFailover)
}

func TestManager_AllUpstreamsFail(t *testing.T) {
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s1.Close()
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s2.Close()

	mgr := testManager(t,
		testUpstream{id: "u1", name: "U1", url: s1.URL, priority: 1},
		testUpstream{id: "u2", name: "U2", url: s2.URL, priority: 2},
	)

	_, err := mgr.CallWithFailover(context.Background(), "POST", "/api/v1/test", []byte("{}"), "dsp_x")
	assert.Error(t, err)
}

func TestManager_SuccessNoFailover(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"passed": true, "score": 90, "conclusion": "PASS"})
	}))
	defer srv.Close()

	mgr := testManager(t,
		testUpstream{id: "ok", name: "OK", url: srv.URL, priority: 1},
		testUpstream{id: "backup", name: "Backup", url: "http://127.0.0.1:1", priority: 2},
	)

	result, err := mgr.CallWithFailover(context.Background(), "POST", "/api/v1/test", []byte("{}"), "dsp_ok")
	require.NoError(t, err)
	assert.Equal(t, "ok", result.UpstreamID)
	assert.False(t, result.UsedFailover)
}

func TestManager_Status(t *testing.T) {
	mgr := testManager(t,
		testUpstream{id: "u1", name: "U1", url: "http://127.0.0.1:1", priority: 1},
		testUpstream{id: "u2", name: "U2", url: "http://127.0.0.1:2", priority: 2},
	)
	status := mgr.Status()
	assert.Len(t, status, 2)
	assert.Equal(t, "closed", status[0].State)
}

func TestManager_ResetCircuit(t *testing.T) {
	mgr := testManager(t, testUpstream{id: "u1", name: "U1", url: "http://127.0.0.1:1", priority: 1})
	mgr.breakers["u1"].RecordFailure()
	mgr.breakers["u1"].RecordFailure()
	mgr.breakers["u1"].RecordFailure()

	err := mgr.ResetCircuit("u1")
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, mgr.breakers["u1"].State())
}

func TestManager_ResetCircuitNotFound(t *testing.T) {
	mgr := testManager(t, testUpstream{id: "u1", name: "U1", url: "http://127.0.0.1:1", priority: 1})
	err := mgr.ResetCircuit("nonexistent")
	assert.Error(t, err)
}

func TestTraceStore_RecordAndList(t *testing.T) {
	ts := NewTraceStore(10)
	ts.Record(TraceRecord{UpstreamID: "u1", DispatchID: "d1"})
	ts.Record(TraceRecord{UpstreamID: "u2", DispatchID: "d2"})
	ts.Record(TraceRecord{UpstreamID: "u1", DispatchID: "d3"})

	all := ts.List(context.Background(), "", 10)
	assert.Len(t, all, 3)

	u1 := ts.List(context.Background(), "u1", 10)
	assert.Len(t, u1, 2)

	assert.Equal(t, 3, ts.Count(context.Background()))
}

func TestTraceStore_RingBuffer(t *testing.T) {
	ts := NewTraceStore(3)
	for i := 0; i < 10; i++ {
		ts.Record(TraceRecord{UpstreamID: fmt.Sprintf("u%d", i)})
	}
	assert.Equal(t, 3, ts.Count(context.Background()), "should cap at maxLen")
}

func TestManager_ConcurrentCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"passed": true, "score": 90, "conclusion": "PASS"})
	}))
	defer srv.Close()

	mgr := testManager(t, testUpstream{id: "ok", name: "OK", url: srv.URL, priority: 1})

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mgr.CallWithFailover(context.Background(), "POST", "/api/v1/test", []byte("{}"), "dsp")
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		assert.NoError(t, err)
	}
}
