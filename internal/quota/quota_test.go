package quota

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/repository"
)

// fakeClock is a controllable clock.Clock for quota tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}
func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
func (f *fakeClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (f *fakeClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (f *fakeClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (f *fakeClock) Since(t time.Time) time.Duration        { return f.Now().Sub(t) }

type noopAudit struct{}

func (noopAudit) Record(context.Context, string, string, string, string, string) error { return nil }

func newTestService(t *testing.T, maxActive int, retention time.Duration) (*Service, *repository.Store, *persistence.KVStore, *fakeClock, string) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	if err := kv.Open(context.Background()); err != nil {
		t.Fatalf("open kv: %v", err)
	}
	t.Cleanup(func() { kv.Close() })
	store := repository.NewStore(kv)
	clk := newFakeClock()
	policy := Policy{MaxActivePerEnterprise: maxActive, ReservationRetention: retention}
	svc := NewService(store, repository.NewSubmissionRepo(store), noopAudit{}, logging.NewNop(), clk, policy)
	return svc, store, kv, clk, dir
}

func makeSubmission(t *testing.T, rep *repository.SubmissionRepo, id, enterpriseID string, status domain.SubmissionStatus) *domain.Submission {
	t.Helper()
	sub := &domain.Submission{
		ID:           id,
		EnterpriseID: enterpriseID,
		ModelNo:      "M-" + id,
		BatchNo:      "B-" + id,
		Status:       status,
	}
	if err := rep.Save(context.Background(), sub); err != nil {
		t.Fatalf("save submission: %v", err)
	}
	return sub
}

func TestQuota_AcquireAndReleaseBalance(t *testing.T) {
	svc, store, _, _, _ := newTestService(t, 3, time.Hour)
	ctx := context.Background()
	rep := repository.NewSubmissionRepo(store)

	res, err := svc.Acquire(ctx, "ent_A")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if count, _ := svc.CountActive(ctx, "ent_A"); count != 1 {
		t.Fatalf("active count = %d, want 1", count)
	}
	makeSubmission(t, rep, "sub_1", "ent_A", domain.SubmissionPending)
	if err := svc.Bind(ctx, res.ID, "sub_1", "M-sub_1"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := svc.ReleaseBySubmission(ctx, "sub_1", "completed"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if count, _ := svc.CountActive(ctx, "ent_A"); count != 0 {
		t.Fatalf("active count after release = %d, want 0", count)
	}
}

func TestQuota_OverQuotaExceededErrorChain(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 2, time.Hour)
	ctx := context.Background()
	if _, err := svc.Acquire(ctx, "ent_A"); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if _, err := svc.Acquire(ctx, "ent_A"); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}
	_, err := svc.Acquire(ctx, "ent_A")
	if err == nil {
		t.Fatal("expected quota exceeded")
	}
	if !errors.Is(err, errorsx.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded chain, got %v", err)
	}
}

func TestQuota_IdempotentRelease(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 2, time.Hour)
	ctx := context.Background()
	res, err := svc.Acquire(ctx, "ent_A")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := svc.Release(ctx, res.ID, "done"); err != nil {
		t.Fatalf("release 1: %v", err)
	}
	if err := svc.Release(ctx, res.ID, "done"); err != nil {
		t.Fatalf("idempotent release should be no-op, got %v", err)
	}
	if count, _ := svc.CountActive(ctx, "ent_A"); count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestQuota_ConcurrentAcquireRespectsLimit(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 3, time.Hour)
	ctx := context.Background()
	var wg sync.WaitGroup
	var success, denied int64
	const goroutines = 50
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.Acquire(ctx, "ent_RACE")
			if err == nil {
				atomic.AddInt64(&success, 1)
			} else if errors.Is(err, errorsx.ErrQuotaExceeded) {
				atomic.AddInt64(&denied, 1)
			}
		}()
	}
	wg.Wait()
	if success != 3 {
		t.Fatalf("success = %d, want exactly 3", success)
	}
	if denied != goroutines-3 {
		t.Fatalf("denied = %d, want %d", denied, goroutines-3)
	}
	count, _ := svc.CountActive(ctx, "ent_RACE")
	if count != 3 {
		t.Fatalf("active count = %d, want 3", count)
	}
}

func TestQuota_RestartRecomputeReleasesTerminal(t *testing.T) {
	svc, store, _, _, _ := newTestService(t, 5, time.Hour)
	ctx := context.Background()
	rep := repository.NewSubmissionRepo(store)
	res, err := svc.Acquire(ctx, "ent_A")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	makeSubmission(t, rep, "sub_t", "ent_A", domain.SubmissionPending)
	svc.Bind(ctx, res.ID, "sub_t", "M")
	// Simulate the submission reaching a terminal state out-of-band.
	sub, _, _ := rep.Get(ctx, "sub_t")
	sub.Status = domain.SubmissionCompleted
	rep.Save(ctx, sub)

	if count, _ := svc.CountActive(ctx, "ent_A"); count != 1 {
		t.Fatalf("pre-recompute count = %d, want 1", count)
	}
	n, err := svc.Recompute(ctx)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if n != 1 {
		t.Fatalf("reconciled = %d, want 1", n)
	}
	if count, _ := svc.CountActive(ctx, "ent_A"); count != 0 {
		t.Fatalf("post-recompute count = %d, want 0", count)
	}
}

func TestQuota_RestartRecomputeReleasesMissingSubmission(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 5, time.Hour)
	ctx := context.Background()
	res, err := svc.Acquire(ctx, "ent_A")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Bind to a submission id that was never persisted (crash before save).
	svc.Bind(ctx, res.ID, "ghost_sub", "M")
	n, err := svc.Recompute(ctx)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if n != 1 {
		t.Fatalf("reconciled = %d, want 1", n)
	}
}

func TestQuota_PersistAcrossRestartRecovery(t *testing.T) {
	svc, store, kv, clk, dir := newTestService(t, 5, time.Hour)
	ctx := context.Background()
	rep := repository.NewSubmissionRepo(store)
	for i := 0; i < 3; i++ {
		res, _ := svc.Acquire(ctx, "ent_A")
		subID := "sub_pr_" + string(rune('1'+i))
		makeSubmission(t, rep, subID, "ent_A", domain.SubmissionPending)
		svc.Bind(ctx, res.ID, subID, "M")
	}
	if count, _ := svc.CountActive(ctx, "ent_A"); count != 3 {
		t.Fatalf("pre-restart count = %d, want 3", count)
	}
	// Simulate process restart: close and reopen on the same data dir.
	kv.Close()
	kv2 := persistence.NewKVStore(dir)
	if err := kv2.Open(ctx); err != nil {
		t.Fatalf("reopen kv: %v", err)
	}
	t.Cleanup(func() { kv2.Close() })
	store2 := repository.NewStore(kv2)
	svc2 := NewService(store2, repository.NewSubmissionRepo(store2), noopAudit{}, logging.NewNop(), clk, Policy{MaxActivePerEnterprise: 5, ReservationRetention: time.Hour})
	if count, _ := svc2.CountActive(ctx, "ent_A"); count != 3 {
		t.Fatalf("post-restart count = %d, want 3 (must recover from disk)", count)
	}
}

func TestQuota_SweepExpiredDeletesOldReleased(t *testing.T) {
	svc, _, _, clk, _ := newTestService(t, 2, 2*time.Hour)
	ctx := context.Background()
	res, _ := svc.Acquire(ctx, "ent_A")
	if err := svc.Release(ctx, res.ID, "done"); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Not yet expired.
	if n, _ := svc.SweepExpired(ctx); n != 0 {
		t.Fatalf("sweep before retention = %d, want 0", n)
	}
	clk.advance(3 * time.Hour)
	if n, _ := svc.SweepExpired(ctx); n != 1 {
		t.Fatalf("sweep after retention = %d, want 1", n)
	}
}

func TestQuota_ListUsagePaginationBoundary(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 5, time.Hour)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		svc.Acquire(ctx, "ent_"+string(rune('A'+i)))
	}
	page1, total, err := svc.ListUsage(ctx, domain.PaginationParams{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 || len(page1) != 2 {
		t.Fatalf("page1 total=%d len=%d, want 5/2", total, len(page1))
	}
	page3, _, _ := svc.ListUsage(ctx, domain.PaginationParams{Page: 3, PageSize: 2})
	if len(page3) != 1 {
		t.Fatalf("last page len = %d, want 1", len(page3))
	}
	page4, _, _ := svc.ListUsage(ctx, domain.PaginationParams{Page: 4, PageSize: 2})
	if len(page4) != 0 {
		t.Fatalf("past-end page len = %d, want 0", len(page4))
	}
}

func TestQuota_GetUsageRemaining(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 4, time.Hour)
	ctx := context.Background()
	svc.Acquire(ctx, "ent_A")
	svc.Acquire(ctx, "ent_A")
	u, err := svc.GetUsage(ctx, "ent_A")
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	if u.ActiveCount != 2 || u.MaxActive != 4 || u.Remaining() != 2 {
		t.Fatalf("usage = %+v, want 2/4/2", u)
	}
}

func TestQuota_AcquireEmptyEnterpriseRejected(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 4, time.Hour)
	_, err := svc.Acquire(context.Background(), "")
	if err == nil || !errors.Is(err, errorsx.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestQuota_ReleaseMissingNotFound(t *testing.T) {
	svc, _, _, _, _ := newTestService(t, 4, time.Hour)
	err := svc.Release(context.Background(), "qta_missing", "x")
	if err == nil || !errors.Is(err, errorsx.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

// ensure clock satisfies the interface at compile time.
var _ clock.Clock = (*fakeClock)(nil)
var _ = filepath.Join
