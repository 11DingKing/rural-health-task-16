package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TraceRecord stores one upstream call's request/response for audit.
type TraceRecord struct {
	ID           string    `json:"id"`
	UpstreamID   string    `json:"upstream_id"`
	DispatchID   string    `json:"dispatch_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
	StatusCode   int       `json:"status_code"`
	Duration     string    `json:"duration"`
	Error        string    `json:"error,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// TraceStore keeps upstream call traces in memory with a ring buffer.
// In production this would be persisted, but it's kept volatile since
// the primary business data is in the KV store.
type TraceStore struct {
	mu     sync.Mutex
	traces []TraceRecord
	maxLen int
}

func NewTraceStore(maxLen int) *TraceStore {
	if maxLen <= 0 {
		maxLen = 1000
	}
	return &TraceStore{maxLen: maxLen}
}

func (ts *TraceStore) Record(tr TraceRecord) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if tr.ID == "" {
		tr.ID = "trc_" + uuid.NewString()
	}
	ts.traces = append(ts.traces, tr)
	if len(ts.traces) > ts.maxLen {
		ts.traces = ts.traces[len(ts.traces)-ts.maxLen:]
	}
}

func (ts *TraceStore) List(ctx context.Context, upstreamID string, limit int) []TraceRecord {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if limit <= 0 || limit > len(ts.traces) {
		limit = len(ts.traces)
	}
	var results []TraceRecord
	for i := len(ts.traces) - 1; i >= 0 && len(results) < limit; i-- {
		if upstreamID == "" || ts.traces[i].UpstreamID == upstreamID {
			results = append(results, ts.traces[i])
		}
	}
	return results
}

func (ts *TraceStore) Count(ctx context.Context) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.traces)
}
