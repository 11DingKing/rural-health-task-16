package sla

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/repository"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (f *fakeClock) Now() time.Time                         { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
func (f *fakeClock) advance(d time.Duration)                { f.mu.Lock(); defer f.mu.Unlock(); f.now = f.now.Add(d) }
func (f *fakeClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (f *fakeClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (f *fakeClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (f *fakeClock) Since(t time.Time) time.Duration        { return f.Now().Sub(t) }

type noopAudit struct{}

func (noopAudit) Record(context.Context, string, string, string, string, string) error { return nil }

func newTestService(t *testing.T, policy Policy) (*Service, *repository.Store, *persistence.KVStore, *fakeClock, string) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	if err := kv.Open(context.Background()); err != nil {
		t.Fatalf("open kv: %v", err)
	}
	t.Cleanup(func() { kv.Close() })
	store := repository.NewStore(kv)
	clk := newFakeClock()
	dispatchRepo := repository.NewDispatchRepo(store)
	svc := NewService(store, dispatchRepo, noopAudit{}, logging.NewNop(), clk, policy)
	return svc, store, kv, clk, dir
}

func makeDispatch(t *testing.T, rep *repository.DispatchRepo, id, standard string, assignedAt time.Time) *domain.DispatchTask {
	t.Helper()
	task := &domain.DispatchTask{
		ID:           id,
		SubmissionID: "sub_" + id,
		StandardCode: standard,
		AgencyID:     "agency_A",
		Status:       domain.DispatchAssigned,
		AssignedAt:   assignedAt,
	}
	if err := rep.Save(context.Background(), task); err != nil {
		t.Fatalf("save dispatch: %v", err)
	}
	return task
}

func TestSLA_EnsureIdempotent(t *testing.T) {
	svc, store, _, clk, _ := newTestService(t, DefaultPolicy())
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_1", "GB-9706", clk.Now())

	rec1, err := svc.Ensure(context.Background(), task)
	if err != nil {
		t.Fatalf("ensure 1: %v", err)
	}
	rec2, err := svc.Ensure(context.Background(), task)
	if err != nil {
		t.Fatalf("ensure 2: %v", err)
	}
	if rec1.ID != rec2.ID {
		t.Fatalf("ensure not idempotent: %s != %s", rec1.ID, rec2.ID)
	}
	if rec1.Escalation != LevelOnTime {
		t.Fatalf("new record should be on_time, got %s", rec1.Escalation)
	}
}

func TestSLA_ValidTransitionsWarningThenBreach(t *testing.T) {
	policy := DefaultPolicy()
	svc, store, _, clk, _ := newTestService(t, policy)
	rep := repository.NewDispatchRepo(store)
	start := clk.Now()
	task := makeDispatch(t, rep, "dsp_2", "GB-9706", start)

	rec, _ := svc.Ensure(context.Background(), task)
	duration := policy.DurationFor("GB-9706")

	clk.advance(time.Duration(float64(duration) * 0.8)) // past warning, before deadline
	rec, err := svc.Evaluate(context.Background(), "dsp_2")
	if err != nil {
		t.Fatalf("evaluate warning: %v", err)
	}
	if rec.Escalation != LevelWarning {
		t.Fatalf("expected warning, got %s", rec.Escalation)
	}

	clk.advance(duration) // past deadline
	rec, err = svc.Evaluate(context.Background(), "dsp_2")
	if err != nil {
		t.Fatalf("evaluate breach: %v", err)
	}
	if rec.Escalation != LevelBreached {
		t.Fatalf("expected breached, got %s", rec.Escalation)
	}
}

func TestSLA_InvalidTransitionRejectEscalateOnTime(t *testing.T) {
	svc, store, _, clk, _ := newTestService(t, DefaultPolicy())
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_3", "GB-9706", clk.Now())
	svc.Ensure(context.Background(), task)

	_, err := svc.Escalate(context.Background(), "dsp_3")
	if err == nil {
		t.Fatal("escalating on_time must be rejected")
	}
	if !errors.Is(err, errorsx.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition chain, got %v", err)
	}
}

func TestSLA_ScanBreachesEscalates(t *testing.T) {
	policy := DefaultPolicy()
	svc, store, _, clk, _ := newTestService(t, policy)
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_4", "GB-9706", clk.Now())
	svc.Ensure(context.Background(), task)

	clk.advance(policy.DurationFor("GB-9706") * 2)
	svc.Evaluate(context.Background(), "dsp_4")

	if c, _ := svc.CountByEscalation(context.Background(), LevelBreached); c != 1 {
		t.Fatalf("breached count = %d, want 1", c)
	}
	n, err := svc.ScanBreaches(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("escalated = %d, want 1", n)
	}
	if c, _ := svc.CountByEscalation(context.Background(), LevelEscalated); c != 1 {
		t.Fatalf("escalated count = %d, want 1", c)
	}
	// Scan again is idempotent: nothing left to escalate.
	if n, _ := svc.ScanBreaches(context.Background()); n != 0 {
		t.Fatalf("second scan = %d, want 0", n)
	}
}

func TestSLA_IdempotentEvaluate(t *testing.T) {
	svc, store, _, clk, _ := newTestService(t, DefaultPolicy())
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_5", "GB-9706", clk.Now())
	rec, _ := svc.Ensure(context.Background(), task)
	before := rec.Escalation
	rec2, _ := svc.Evaluate(context.Background(), "dsp_5")
	if rec2.Escalation != before {
		t.Fatalf("evaluate without time change should be no-op, got %s", rec2.Escalation)
	}
}

func TestSLA_RestartRecomputeBackfillsAndReevaluates(t *testing.T) {
	policy := DefaultPolicy()
	dir := t.TempDir()

	// First process: create an overdue dispatch with no SLA record.
	kv := persistence.NewKVStore(dir)
	if err := kv.Open(context.Background()); err != nil {
		t.Fatalf("open kv: %v", err)
	}
	store := repository.NewStore(kv)
	rep := repository.NewDispatchRepo(store)
	clk := newFakeClock()
	past := clk.Now().Add(-3 * policy.DurationFor("GB-9706"))
	makeDispatch(t, rep, "dsp_rec", "GB-9706", past)
	kv.Close()

	// Restart: reopen the same data dir. Recompute must backfill the
	// missing record from the persisted dispatch and breach it.
	kv2 := persistence.NewKVStore(dir)
	if err := kv2.Open(context.Background()); err != nil {
		t.Fatalf("reopen kv: %v", err)
	}
	t.Cleanup(func() { kv2.Close() })
	store2 := repository.NewStore(kv2)
	svc := NewService(store2, repository.NewDispatchRepo(store2), noopAudit{}, logging.NewNop(), clk, policy)

	n, err := svc.Recompute(context.Background())
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if n < 1 {
		t.Fatalf("recompute affected = %d, want >=1", n)
	}
	rec, ok, err := svc.GetByDispatch(context.Background(), "dsp_rec")
	if err != nil || !ok {
		t.Fatalf("recompute must backfill the sla record from disk: ok=%v err=%v", ok, err)
	}
	if rec.Escalation != LevelBreached {
		t.Fatalf("recompute should breach the overdue record, got %s", rec.Escalation)
	}
}

func TestSLA_PersistAcrossRestartRecovery(t *testing.T) {
	policy := DefaultPolicy()
	svc, store, kv, clk, dir := newTestService(t, policy)
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_p", "GB-9706", clk.Now())
	rec, _ := svc.Ensure(context.Background(), task)

	kv.Close()
	kv2 := persistence.NewKVStore(dir)
	if err := kv2.Open(context.Background()); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { kv2.Close() })
	store2 := repository.NewStore(kv2)
	svc2 := NewService(store2, repository.NewDispatchRepo(store2), noopAudit{}, logging.NewNop(), clk, policy)
	got, ok, err := svc2.GetByDispatch(context.Background(), "dsp_p")
	if err != nil || !ok {
		t.Fatalf("must recover record from disk: ok=%v err=%v", ok, err)
	}
	if got.ID != rec.ID {
		t.Fatalf("recovered id = %s, want %s", got.ID, rec.ID)
	}
}

func TestSLA_MarkResolvedTerminalAndIllegalFromResolved(t *testing.T) {
	svc, store, _, clk, _ := newTestService(t, DefaultPolicy())
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_r", "GB-9706", clk.Now())
	svc.Ensure(context.Background(), task)

	if err := svc.MarkResolved(context.Background(), "dsp_r"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rec, _, _ := svc.GetByDispatch(context.Background(), "dsp_r")
	if rec.Escalation != LevelResolved {
		t.Fatalf("expected resolved, got %s", rec.Escalation)
	}
	// Evaluate must be a no-op on a terminal record.
	rec2, _ := svc.Evaluate(context.Background(), "dsp_r")
	if rec2.Escalation != LevelResolved {
		t.Fatalf("terminal record must not change, got %s", rec2.Escalation)
	}
	// resolved -> cancelled is illegal.
	err := svc.MarkCancelled(context.Background(), "dsp_r")
	if err == nil || !errors.Is(err, errorsx.ErrInvalidTransition) {
		t.Fatalf("resolved->cancelled must be rejected, got %v", err)
	}
}

func TestSLA_ListBreachesPaginationBoundary(t *testing.T) {
	policy := DefaultPolicy()
	svc, store, _, clk, _ := newTestService(t, policy)
	rep := repository.NewDispatchRepo(store)
	for i := 0; i < 5; i++ {
		task := &domain.DispatchTask{
			ID:           "dsp_lb_" + string(rune('1'+i)),
			SubmissionID: "sub",
			StandardCode: "GB-9706",
			AgencyID:     "agency_A",
			Status:       domain.DispatchAssigned,
			AssignedAt:   clk.Now().Add(-time.Duration(i+1) * policy.DurationFor("GB-9706")),
		}
		rep.Save(context.Background(), task)
		svc.Ensure(context.Background(), task)
		svc.Evaluate(context.Background(), task.ID)
	}
	page1, total, err := svc.ListBreaches(context.Background(), domain.PaginationParams{Page: 1, PageSize: 2}, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 || len(page1) != 2 {
		t.Fatalf("total=%d len=%d, want 5/2", total, len(page1))
	}
	page3, _, _ := svc.ListBreaches(context.Background(), domain.PaginationParams{Page: 3, PageSize: 2}, "")
	if len(page3) != 1 {
		t.Fatalf("last page len = %d, want 1", len(page3))
	}
}

func TestSLA_ErrorChainInvalidTransition(t *testing.T) {
	svc, store, _, clk, _ := newTestService(t, DefaultPolicy())
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_e", "GB-9706", clk.Now())
	svc.Ensure(context.Background(), task)
	_, err := svc.Escalate(context.Background(), "dsp_e")
	if !errors.Is(err, errorsx.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition chain, got %v", err)
	}
}

func TestSLA_ConcurrentEvaluateSafe(t *testing.T) {
	policy := DefaultPolicy()
	svc, store, _, clk, _ := newTestService(t, policy)
	rep := repository.NewDispatchRepo(store)
	task := makeDispatch(t, rep, "dsp_c", "GB-9706", clk.Now())
	svc.Ensure(context.Background(), task)
	clk.advance(policy.DurationFor("GB-9706") * 2)

	var wg sync.WaitGroup
	const n = 30
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.Evaluate(context.Background(), "dsp_c")
		}()
	}
	wg.Wait()
	rec, _, _ := svc.GetByDispatch(context.Background(), "dsp_c")
	if rec.Escalation != LevelBreached {
		t.Fatalf("concurrent evaluate should settle at breached, got %s", rec.Escalation)
	}
}

var _ clock.Clock = (*fakeClock)(nil)
