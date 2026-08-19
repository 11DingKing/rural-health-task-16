package consistency

import (
	"context"
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

type testClock struct{ *clock.Mock }

func (t testClock) Now() time.Time                         { return t.Mock.Now() }
func (t testClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (t testClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (t testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t testClock) Since(t2 time.Time) time.Duration       { return t.Mock.Now().Sub(t2) }

func newTestConsistencyService(t *testing.T) (*Service, *persistence.KVStore) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })

	store := repository.NewStore(kv)
	summaryRepo := repository.NewSummaryRepo(store)
	resultRepo := repository.NewResultRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	subRepo := repository.NewSubmissionRepo(store)
	clk := testClock{clock.NewMock()}
	svc := NewService(summaryRepo, resultRepo, dispatchRepo, subRepo, logging.NewNop(), clk)
	return svc, kv
}

func makeSubmission(t *testing.T, subRepo *repository.SubmissionRepo, clk clock.Clock) *domain.Submission {
	sub := &domain.Submission{
		ID: "sub_1", EnterpriseID: "ent_1", EnterpriseName: "E",
		ModelNo: "EXO-1", ModelName: "Test", Category: domain.CategoryExoskeleton,
		RiskLevel: domain.RiskLevelMedium, StandardCodes: []string{"GB-9706", "YY-0505"},
		BatchNo: "B1", Status: domain.SubmissionTesting,
		IdempotencyKey: "sub_EXO-1:B1",
		Audit:          domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()},
	}
	require.NoError(t, subRepo.Save(context.Background(), sub))
	return sub
}

func TestConsistency_RecomputeSummary(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	for _, std := range sub.StandardCodes {
		task := &domain.DispatchTask{
			ID: "dsp_" + std, SubmissionID: sub.ID, ModelNo: sub.ModelNo,
			BatchNo: sub.BatchNo, StandardCode: std, AgencyID: "agn_1",
			Status: domain.DispatchCompleted, AssignedAt: clk.Now(),
			Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()},
		}
		require.NoError(t, dispatchRepo.Save(context.Background(), task))
	}

	for _, std := range sub.StandardCodes {
		result := &domain.TestResult{
			ID: "res_" + std, DispatchID: "dsp_" + std, SubmissionID: sub.ID,
			ModelNo: sub.ModelNo, StandardCode: std, AgencyID: "agn_1",
			Passed: true, Score: 95, Conclusion: "PASS",
			TestDate: clk.Now(), ReceivedAt: clk.Now(),
			Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()},
		}
		require.NoError(t, resultRepo.Save(context.Background(), result))
	}

	summary, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.True(t, summary.OverallPassed)
	assert.Equal(t, 2, summary.PassedCount)
	assert.Equal(t, 0, summary.FailedCount)
	assert.Equal(t, domain.SummaryComputed, summary.Status)
}

func TestConsistency_RecomputeWithFailures(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	task1 := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	task2 := &domain.DispatchTask{ID: "dsp_2", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "YY-0505", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(context.Background(), task1))
	require.NoError(t, dispatchRepo.Save(context.Background(), task2))

	res1 := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	res2 := &domain.TestResult{ID: "r2", DispatchID: "dsp_2", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "YY-0505", AgencyID: "a1", Passed: false, Score: 30, Conclusion: "FAIL", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(context.Background(), res1))
	require.NoError(t, resultRepo.Save(context.Background(), res2))

	summary, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.False(t, summary.OverallPassed)
	assert.Equal(t, 1, summary.PassedCount)
	assert.Equal(t, 1, summary.FailedCount)
}

func TestConsistency_RecomputeVersionIncrement(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	task := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(context.Background(), task))
	res := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(context.Background(), res))

	summary1, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, summary1.Version)

	summary2, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, summary2.Version, "version should increment on recompute")
}

func TestConsistency_CheckConsistencyOK(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	task := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(context.Background(), task))
	res := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(context.Background(), res))

	_, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)

	check, err := svc.CheckConsistency(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.True(t, check.Consistent)
}

func TestConsistency_FreezeForReview(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	summaryRepo := repository.NewSummaryRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	task := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(context.Background(), task))
	res := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(context.Background(), res))

	_, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)

	err = svc.FreezeForReview(context.Background(), sub.ModelNo, "test freeze")
	require.NoError(t, err)

	updated, err := svc.GetSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SummaryFrozen, updated.Status)
	_ = summaryRepo
}

func TestConsistency_MarkStaleAndRepair(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	clk := testClock{clock.NewMock()}

	sub := makeSubmission(t, subRepo, clk)
	task := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(context.Background(), task))
	res := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(context.Background(), res))

	summary, err := svc.RecomputeSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SummaryComputed, summary.Status)

	require.NoError(t, svc.MarkStale(context.Background(), sub.ID))
	staleSummary, err := svc.GetSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SummaryStale, staleSummary.Status)

	repaired, err := svc.RepairStale(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)

	fixedSummary, err := svc.GetSummary(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SummaryComputed, fixedSummary.Status, "should be recomputed after repair")
}

func TestConsistency_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := repository.NewStore(kv)
	subRepo := repository.NewSubmissionRepo(store)
	dispatchRepo := repository.NewDispatchRepo(store)
	resultRepo := repository.NewResultRepo(store)
	summaryRepo := repository.NewSummaryRepo(store)
	clk := testClock{clock.NewMock()}
	svc := NewService(summaryRepo, resultRepo, dispatchRepo, subRepo, logging.NewNop(), clk)

	sub := makeSubmission(t, subRepo, clk)
	task := &domain.DispatchTask{ID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, BatchNo: sub.BatchNo, StandardCode: "GB-9706", AgencyID: "a1", Status: domain.DispatchCompleted, AssignedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, dispatchRepo.Save(ctx, task))
	res := &domain.TestResult{ID: "r1", DispatchID: "dsp_1", SubmissionID: sub.ID, ModelNo: sub.ModelNo, StandardCode: "GB-9706", AgencyID: "a1", Passed: true, Score: 90, Conclusion: "PASS", TestDate: clk.Now(), ReceivedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
	require.NoError(t, resultRepo.Save(ctx, res))
	summary, err := svc.RecomputeSummary(ctx, sub.ID)
	require.NoError(t, err)
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := repository.NewStore(kv2)
	subRepo2 := repository.NewSubmissionRepo(store2)
	dispatchRepo2 := repository.NewDispatchRepo(store2)
	resultRepo2 := repository.NewResultRepo(store2)
	summaryRepo2 := repository.NewSummaryRepo(store2)
	svc2 := NewService(summaryRepo2, resultRepo2, dispatchRepo2, subRepo2, logging.NewNop(), clk)

	recovered, err := svc2.GetSummary(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, summary.ID, recovered.ID)
	assert.Equal(t, summary.OverallPassed, recovered.OverallPassed)
	assert.Equal(t, domain.SummaryComputed, recovered.Status)
}

func TestConsistency_InvalidTransitionReject(t *testing.T) {
	ssm := SummaryStateMachine{}
	err := ssm.Transition(domain.SummaryFinal, domain.SummaryPending)
	assert.Error(t, err)
	err = ssm.Transition(domain.SummaryFrozen, domain.SummaryFinal)
	assert.Error(t, err)
}

func TestConsistency_GetSummaryNotFound(t *testing.T) {
	svc, _ := newTestConsistencyService(t)
	_, err := svc.GetSummary(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestConsistency_ListSummaries(t *testing.T) {
	svc, kv := newTestConsistencyService(t)
	store := repository.NewStore(kv)
	summaryRepo := repository.NewSummaryRepo(store)
	clk := testClock{clock.NewMock()}

	for i := 0; i < 3; i++ {
		s := &domain.Summary{ID: "sum_" + string(rune('A'+i)), ModelNo: "M" + string(rune('A'+i)), SubmissionID: "sub_" + string(rune('A'+i)), Status: domain.SummaryComputed, Version: 1, ComputedAt: clk.Now(), Audit: domain.AuditMeta{CreatedAt: clk.Now(), UpdatedAt: clk.Now()}}
		require.NoError(t, summaryRepo.Save(context.Background(), s))
	}

	sums, total, err := svc.ListSummaries(context.Background(), domain.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, sums, 3)
}
