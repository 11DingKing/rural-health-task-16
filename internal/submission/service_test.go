package submission

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/repository"
)

func newTestService(t *testing.T) (*Service, *persistence.KVStore) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })

	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	ruleRepo := repository.NewRuleRepo(store)
	idemRepo := repository.NewIdempotencyRepo(store)

	mockClock := clock.NewMock()
	clk := realClockAdapter{mockClock}
	svc := NewService(subRepo, ruleRepo, idemRepo, logging.NewNop(), clk)
	return svc, kv
}

type realClockAdapter struct{ *clock.Mock }

func (r realClockAdapter) Now() time.Time                         { return r.Mock.Now() }
func (r realClockAdapter) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (r realClockAdapter) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (r realClockAdapter) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (r realClockAdapter) Since(t time.Time) time.Duration        { return r.Mock.Now().Sub(t) }

func makeCreateReq() CreateRequest {
	return CreateRequest{
		EnterpriseID:   "ent_001",
		EnterpriseName: "Test Rehab Co",
		ModelNo:        "EXO-R1",
		ModelName:      "Rehab Exoskeleton V1",
		Category:       domain.CategoryExoskeleton,
		RiskLevel:      domain.RiskLevelMedium,
		StandardCodes:  []string{"GB-9706", "YY-0505"},
		BatchNo:        "BATCH-001",
	}
}

func TestService_CreateSubmission(t *testing.T) {
	svc, _ := newTestService(t)
	sub, isDup, err := svc.Create(context.Background(), makeCreateReq())
	require.NoError(t, err)
	assert.False(t, isDup)
	assert.NotEmpty(t, sub.ID)
	assert.Equal(t, domain.SubmissionPending, sub.Status)
	assert.Equal(t, "EXO-R1", sub.ModelNo)
	assert.False(t, sub.Charge.IsDuplicate)
	assert.Greater(t, sub.Charge.Amount.String(), "0")
}

func TestService_IdempotentDuplicateReplay(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub1, isDup1, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)
	assert.False(t, isDup1)

	sub2, isDup2, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)
	assert.True(t, isDup2, "duplicate submission should be detected")
	assert.Equal(t, sub1.ID, sub2.ID, "should return same submission")
}

func TestService_IdempotentDifferentBatch(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	req1 := makeCreateReq()
	req1.BatchNo = "BATCH-001"
	sub1, _, err := svc.Create(ctx, req1)
	require.NoError(t, err)

	req2 := makeCreateReq()
	req2.BatchNo = "BATCH-002"
	sub2, isDup, err := svc.Create(ctx, req2)
	require.NoError(t, err)
	assert.False(t, isDup)
	assert.NotEqual(t, sub1.ID, sub2.ID)
}

func TestService_IdempotentDifferentModel(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	req1 := makeCreateReq()
	req1.ModelNo = "EXO-R1"
	sub1, _, err := svc.Create(ctx, req1)
	require.NoError(t, err)

	req2 := makeCreateReq()
	req2.ModelNo = "EXO-R2"
	sub2, isDup, err := svc.Create(ctx, req2)
	require.NoError(t, err)
	assert.False(t, isDup)
	assert.NotEqual(t, sub1.ID, sub2.ID)
}

func TestService_ValidationRejectsEmptyFields(t *testing.T) {
	svc, _ := newTestService(t)
	_, _, err := svc.Create(context.Background(), CreateRequest{})
	require.Error(t, err)
}

func TestService_GetSubmission(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	found, err := svc.Get(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, found.ID)
}

func TestService_GetNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestService_CancelSubmission(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	cancelled, err := svc.Cancel(ctx, sub.ID, "enterprise")
	require.NoError(t, err)
	assert.Equal(t, domain.SubmissionCancelled, cancelled.Status)
}

func TestService_CancelInvalidTransition(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	sub.Status = domain.SubmissionCompleted
	_ = svc.submissionRepo.Save(ctx, sub)

	_, err = svc.Cancel(ctx, sub.ID, "enterprise")
	assert.Error(t, err, "cannot cancel completed submission")
}

func TestService_TransitionStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	updated, err := svc.TransitionStatus(ctx, sub.ID, domain.SubmissionDispatched, "system")
	require.NoError(t, err)
	assert.Equal(t, domain.SubmissionDispatched, updated.Status)
}

func TestService_TransitionInvalidReject(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	_, err = svc.TransitionStatus(ctx, sub.ID, domain.SubmissionCompleted, "system")
	assert.Error(t, err, "pending -> completed is invalid")
}

func TestService_UpdateSubmission(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)

	newName := "Updated Name"
	newRisk := domain.RiskLevelHigh
	updated, err := svc.Update(ctx, sub.ID, UpdateRequest{
		ModelName: &newName,
		RiskLevel: &newRisk,
		UpdatedBy: "enterprise",
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.ModelName)
	assert.Equal(t, domain.RiskLevelHigh, updated.RiskLevel)
}

func TestService_ListPaged(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 25; i++ {
		req := makeCreateReq()
		req.ModelNo = "EXO-" + string(rune('A'+i))
		req.BatchNo = "B" + string(rune('A'+i))
		_, _, err := svc.Create(ctx, req)
		require.NoError(t, err)
	}

	params := domain.PaginationParams{Page: 1, PageSize: 10}
	subs, total, err := svc.List(ctx, params, "", "")
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, subs, 10)

	params = domain.PaginationParams{Page: 3, PageSize: 10}
	subs, total, err = svc.List(ctx, params, "", "")
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, subs, 5)
}

func TestService_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	ruleRepo := repository.NewRuleRepo(store)
	idemRepo := repository.NewIdempotencyRepo(store)
	clk := realClockAdapter{clock.NewMock()}
	svc := NewService(subRepo, ruleRepo, idemRepo, logging.NewNop(), clk)

	sub, _, err := svc.Create(ctx, makeCreateReq())
	require.NoError(t, err)
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := repository.NewStore(kv2)
	subRepo2 := repository.NewSubmissionRepo(store2)
	ruleRepo2 := repository.NewRuleRepo(store2)
	idemRepo2 := repository.NewIdempotencyRepo(store2)
	svc2 := NewService(subRepo2, ruleRepo2, idemRepo2, logging.NewNop(), clk)

	recovered, err := svc2.Get(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, recovered.ID)
	assert.Equal(t, sub.ModelNo, recovered.ModelNo)

	_, isDup, err := svc2.Create(ctx, makeCreateReq())
	require.NoError(t, err)
	assert.True(t, isDup, "idempotency should survive restart")
}

func TestService_ConcurrentCreateSameModel(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan *domain.Submission, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, isDup, err := svc.Create(ctx, makeCreateReq())
			if err == nil && sub != nil && !isDup {
				results <- sub
			}
		}()
	}
	wg.Wait()
	close(results)

	count := 0
	for sub := range results {
		count++
		_ = sub
	}
	assert.Equal(t, 1, count, "only one submission should be created for same model+batch")
}

func TestCharge_DuplicateZeroCharge(t *testing.T) {
	cc := NewChargeCalculator(500.0)
	result := cc.Compute(3, true, time.Now())
	assert.Equal(t, "0", result.Amount.String())
	assert.True(t, result.IsDuplicate)
}

func TestCharge_NormalCharge(t *testing.T) {
	cc := NewChargeCalculator(500.0)
	result := cc.Compute(3, false, time.Now())
	assert.Equal(t, "1500", result.Amount.String())
	assert.False(t, result.IsDuplicate)
	assert.Equal(t, 3, result.ItemCount)
}

func TestBusinessKey(t *testing.T) {
	key := BusinessKey("EXO-R1", "BATCH-001")
	assert.Equal(t, "EXO-R1:BATCH-001", key)
}
