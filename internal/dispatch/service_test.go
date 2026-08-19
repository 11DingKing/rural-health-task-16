package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/config"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/gateway"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/persistence"
	"ruralhealth/internal/repository"
)

type testClock struct{ *clock.Mock }

func (t testClock) Now() time.Time                         { return t.Mock.Now() }
func (t testClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (t testClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (t testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t testClock) Since(t2 time.Time) time.Duration       { return t.Mock.Now().Sub(t2) }

func newTestDispatchService(t *testing.T) (*Service, *persistence.KVStore) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })

	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	idemRepo := repository.NewIdempotencyRepo(store)

	mockClock := clock.NewMock()
	clk := testClock{mockClock}
	cfg := config.GatewayConfig{DefaultTimeout: 2 * time.Second, MaxRetries: 3, BaseBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, CircuitMaxFail: 3, CircuitResetAfter: 200 * time.Millisecond}
	ups := []config.UpstreamConfig{{ID: "u1", Name: "U1", BaseURL: "http://127.0.0.1:1", Timeout: time.Second, Priority: 1}}
	gw := gateway.NewManager(cfg, ups, clk.Now)

	svc := NewService(dispatchRepo, subRepo, agencyRepo, resultRepo, idemRepo, gw, logging.NewNop(), clk)
	return svc, kv
}

func newTestSubmission(t *testing.T, subRepo *repository.SubmissionRepo, clk clock.Clock) *domain.Submission {
	sub := &domain.Submission{
		ID: "sub_test_1", EnterpriseID: "ent_1", EnterpriseName: "Test Co",
		ModelNo: "EXO-1", ModelName: "Test", Category: domain.CategoryExoskeleton,
		RiskLevel: domain.RiskLevelMedium, StandardCodes: []string{"GB-9706"},
		BatchNo: "B1", Status: domain.SubmissionPending,
		IdempotencyKey: "sub_EXO-1:B1",
		Audit:          domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()},
	}
	require.NoError(t, subRepo.Save(context.Background(), sub))
	return sub
}

func TestDispatch_DispatchSubmission(t *testing.T) {
	svc, kv := newTestDispatchService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	clk := clock.NewMock()

	agency := &domain.Agency{ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true, Priority: 1, AccreditedStandards: []string{"GB-9706"}, MaxConcurrent: 10}
	require.NoError(t, agencyRepo.Save(context.Background(), agency))

	sub := newTestSubmission(t, subRepo, testClock{clk})
	tasks, err := svc.DispatchSubmission(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, domain.DispatchAssigned, tasks[0].Status)
	assert.Equal(t, "agn_1", tasks[0].AgencyID)
}

func TestDispatch_IdempotentReDispatch(t *testing.T) {
	svc, kv := newTestDispatchService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	clk := clock.NewMock()

	agency := &domain.Agency{ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true, Priority: 1, AccreditedStandards: []string{"GB-9706"}, MaxConcurrent: 10}
	require.NoError(t, agencyRepo.Save(context.Background(), agency))

	sub := newTestSubmission(t, subRepo, testClock{clk})
	tasks1, err := svc.DispatchSubmission(context.Background(), sub.ID)
	require.NoError(t, err)

	tasks2, err := svc.DispatchSubmission(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, len(tasks1), len(tasks2), "re-dispatch should return existing tasks")
}

func TestDispatch_NoAgencyForStandard(t *testing.T) {
	svc, kv := newTestDispatchService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	clk := clock.NewMock()

	sub := newTestSubmission(t, subRepo, testClock{clk})
	_, err := svc.DispatchSubmission(context.Background(), sub.ID)
	assert.Error(t, err)
}

func TestDispatch_SubmissionNotFound(t *testing.T) {
	svc, _ := newTestDispatchService(t)
	_, err := svc.DispatchSubmission(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestDispatch_GetTask(t *testing.T) {
	svc, kv := newTestDispatchService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	clk := clock.NewMock()

	agency := &domain.Agency{ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true, Priority: 1, AccreditedStandards: []string{"GB-9706"}, MaxConcurrent: 10}
	require.NoError(t, agencyRepo.Save(context.Background(), agency))

	sub := newTestSubmission(t, subRepo, testClock{clk})
	tasks, err := svc.DispatchSubmission(context.Background(), sub.ID)
	require.NoError(t, err)

	task, err := svc.Get(context.Background(), tasks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, tasks[0].ID, task.ID)

	_ = dispatchRepo
}

func TestDispatch_GetNotFound(t *testing.T) {
	svc, _ := newTestDispatchService(t)
	_, err := svc.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestDispatch_ListPaged(t *testing.T) {
	svc, kv := newTestDispatchService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	clk := clock.NewMock()

	agency := &domain.Agency{ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true, Priority: 1, AccreditedStandards: []string{"GB-9706"}, MaxConcurrent: 10}
	require.NoError(t, agencyRepo.Save(context.Background(), agency))

	for i := 0; i < 3; i++ {
		sub := &domain.Submission{
			ID: "sub_" + string(rune('A'+i)), EnterpriseID: "ent", EnterpriseName: "E",
			ModelNo: "M" + string(rune('A'+i)), ModelName: "MN", Category: domain.CategoryExoskeleton,
			RiskLevel: domain.RiskLevelMedium, StandardCodes: []string{"GB-9706"},
			BatchNo: "B" + string(rune('A'+i)), Status: domain.SubmissionPending,
			IdempotencyKey: "sub_M" + string(rune('A'+i)) + ":B" + string(rune('A'+i)),
			Audit:          domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()},
		}
		require.NoError(t, subRepo.Save(context.Background(), sub))
		_, err := svc.DispatchSubmission(context.Background(), sub.ID)
		require.NoError(t, err)
	}

	tasks, total, err := svc.ListPaged(context.Background(), domain.PaginationParams{Page: 1, PageSize: 10}, "", "")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, tasks, 3)
}

func TestDispatch_InvalidTransitionReject(t *testing.T) {
	dsm := DispatchStateMachine{}
	err := dsm.Transition(domain.DispatchCreated, domain.DispatchCompleted)
	assert.Error(t, err, "created->completed is invalid")

	err = dsm.Transition(domain.DispatchCompleted, domain.DispatchInProgress)
	assert.Error(t, err, "completed->in_progress is invalid")
}

func TestDispatch_ValidTransition(t *testing.T) {
	dsm := DispatchStateMachine{}
	assert.NoError(t, dsm.Transition(domain.DispatchCreated, domain.DispatchAssigned))
	assert.NoError(t, dsm.Transition(domain.DispatchAssigned, domain.DispatchInProgress))
	assert.NoError(t, dsm.Transition(domain.DispatchInProgress, domain.DispatchCompleted))
}

func TestFailoverConfig_NextRetry(t *testing.T) {
	sel := &AgencySelector{}
	_ = sel
	fh := NewFailoverHandler(sel, FailoverConfig{
		Timeout: time.Second, MaxRetries: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: time.Second,
	}, func() time.Time { return time.Time{} })

	t1 := fh.NextRetryAt(1)
	t2 := fh.NextRetryAt(2)
	t3 := fh.NextRetryAt(3)
	assert.True(t, t2.After(t1))
	assert.True(t, t3.After(t2))
}

func TestDispatch_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	agencyRepo := repository.NewAgencyRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	idemRepo := repository.NewIdempotencyRepo(store)
	clk := testClock{clock.NewMock()}
	cfg := config.GatewayConfig{DefaultTimeout: time.Second, MaxRetries: 3, BaseBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, CircuitMaxFail: 3, CircuitResetAfter: time.Second}
	ups := []config.UpstreamConfig{{ID: "u1", Name: "U1", BaseURL: "http://127.0.0.1:1", Timeout: time.Second, Priority: 1}}
	gw := gateway.NewManager(cfg, ups, clk.Now)
	svc := NewService(dispatchRepo, subRepo, agencyRepo, resultRepo, idemRepo, gw, logging.NewNop(), clk)

	agency := &domain.Agency{ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true, Priority: 1, AccreditedStandards: []string{"GB-9706"}, MaxConcurrent: 10}
	require.NoError(t, agencyRepo.Save(ctx, agency))
	sub := newTestSubmission(t, subRepo, clk)
	tasks, err := svc.DispatchSubmission(ctx, sub.ID)
	require.NoError(t, err)
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := repository.NewStore(kv2)
	dispatchRepo2 := repository.NewDispatchRepo(store2)
	subRepo2 := repository.NewSubmissionRepo(store2)
	agencyRepo2 := repository.NewAgencyRepo(store2)
	resultRepo2 := repository.NewResultRepo(store2)
	idemRepo2 := repository.NewIdempotencyRepo(store2)
	svc2 := NewService(dispatchRepo2, subRepo2, agencyRepo2, resultRepo2, idemRepo2, gw, logging.NewNop(), clk)

	task, err := svc2.Get(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, tasks[0].ID, task.ID)
	assert.Equal(t, domain.DispatchAssigned, task.Status, "status should survive restart")
}
