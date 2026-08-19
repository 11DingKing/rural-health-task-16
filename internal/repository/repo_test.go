package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/persistence"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })
	return NewStore(kv)
}

func TestRepo_SubmissionCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewSubmissionRepo(store)
	ctx := context.Background()

	sub := &domain.Submission{
		ID: "sub_1", EnterpriseID: "ent_1", EnterpriseName: "E",
		ModelNo: "M1", ModelName: "MN", Category: domain.CategoryExoskeleton,
		RiskLevel: domain.RiskLevelMedium, StandardCodes: []string{"S1"},
		BatchNo: "B1", Status: domain.SubmissionPending,
		IdempotencyKey: "sub_M1:B1",
	}
	require.NoError(t, repo.Save(ctx, sub))

	found, ok, err := repo.Get(ctx, "sub_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "M1", found.ModelNo)

	subs, err := repo.FindByModelBatch(ctx, "M1", "B1")
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	subs, err = repo.FindByEnterprise(ctx, "ent_1")
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	subs, err = repo.FindByStatus(ctx, domain.SubmissionPending)
	require.NoError(t, err)
	assert.Len(t, subs, 1)
}

func TestRepo_SubmissionListPaged(t *testing.T) {
	store := newTestStore(t)
	repo := NewSubmissionRepo(store)
	ctx := context.Background()

	for i := 0; i < 25; i++ {
		sub := &domain.Submission{
			ID: "sub_" + string(rune('a'+i)), EnterpriseID: "ent",
			ModelNo: "M" + string(rune('a'+i)), BatchNo: "B" + string(rune('a'+i)),
			Category: domain.CategoryExoskeleton, StandardCodes: []string{"S"},
			Status: domain.SubmissionPending,
		}
		require.NoError(t, repo.Save(ctx, sub))
	}

	subs, total, err := repo.ListPaged(ctx, domain.PaginationParams{Page: 1, PageSize: 10}, "", "")
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, subs, 10)

	subs, total, err = repo.ListPaged(ctx, domain.PaginationParams{Page: 1, PageSize: 10}, "pending", "")
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, subs, 10)
}

func TestRepo_AgencyCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewAgencyRepo(store)
	ctx := context.Background()

	agency := &domain.Agency{
		ID: "agn_1", Code: "AG1", Name: "Agency 1", IsActive: true,
		Priority: 1, AccreditedStandards: []string{"GB-9706", "YY-0505"}, MaxConcurrent: 10,
	}
	require.NoError(t, repo.Save(ctx, agency))

	found, ok, err := repo.Get(ctx, "agn_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "AG1", found.Code)

	agencies, err := repo.FindByStandard(ctx, "GB-9706")
	require.NoError(t, err)
	assert.Len(t, agencies, 1)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	active, err := repo.ListActive(ctx)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

func TestRepo_DispatchCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewDispatchRepo(store)
	ctx := context.Background()

	task := &domain.DispatchTask{
		ID: "dsp_1", SubmissionID: "sub_1", ModelNo: "M1", BatchNo: "B1",
		StandardCode: "GB-9706", AgencyID: "agn_1", AgencyName: "A1",
		Status: domain.DispatchAssigned, Priority: 1, Attempt: 1, MaxAttempts: 3,
	}
	require.NoError(t, repo.Save(ctx, task))

	found, ok, err := repo.Get(ctx, "dsp_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "sub_1", found.SubmissionID)

	tasks, err := repo.FindBySubmission(ctx, "sub_1")
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	tasks, err = repo.FindByAgency(ctx, "agn_1")
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	tasks, err = repo.FindByStatus(ctx, domain.DispatchAssigned)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	tasks, err = repo.FindByModelBatch(ctx, "M1", "B1")
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}

func TestRepo_ResultCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewResultRepo(store)
	ctx := context.Background()

	result := &domain.TestResult{
		ID: "res_1", DispatchID: "dsp_1", SubmissionID: "sub_1",
		ModelNo: "M1", StandardCode: "GB-9706", AgencyID: "agn_1",
		Passed: true, Score: 95, Conclusion: "PASS",
	}
	require.NoError(t, repo.Save(ctx, result))

	found, ok, err := repo.Get(ctx, "res_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, found.Passed)

	results, err := repo.FindByDispatch(ctx, "dsp_1")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = repo.FindBySubmission(ctx, "sub_1")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = repo.FindByModel(ctx, "M1")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestRepo_SummaryCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewSummaryRepo(store)
	ctx := context.Background()

	summary := &domain.Summary{
		ID: "sum_1", ModelNo: "M1", BatchNo: "B1", SubmissionID: "sub_1",
		OverallPassed: true, StandardCount: 2, PassedCount: 2, FailedCount: 0,
		Status: domain.SummaryComputed, Version: 1,
	}
	require.NoError(t, repo.Save(ctx, summary))

	found, ok, err := repo.Get(ctx, "sum_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, found.OverallPassed)

	s, err := repo.FindBySubmission(ctx, "sub_1")
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "sum_1", s.ID)

	sums, err := repo.FindByModel(ctx, "M1")
	require.NoError(t, err)
	assert.Len(t, sums, 1)
}

func TestRepo_CallbackCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewCallbackRepo(store)
	ctx := context.Background()

	cb := &domain.Callback{
		ID: "cb_1", SubmissionID: "sub_1", EnterpriseID: "ent_1",
		WebhookURL: "http://example.com", Payload: "{}", Signature: "sig",
		Status: domain.CallbackPending, Attempt: 0, MaxAttempts: 3,
	}
	require.NoError(t, repo.Save(ctx, cb))

	found, ok, err := repo.Get(ctx, "cb_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, domain.CallbackPending, found.Status)

	cbs, err := repo.FindBySubmission(ctx, "sub_1")
	require.NoError(t, err)
	assert.Len(t, cbs, 1)

	cbs, err = repo.FindByStatus(ctx, domain.CallbackPending)
	require.NoError(t, err)
	assert.Len(t, cbs, 1)
}

func TestRepo_IdempotencyCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewIdempotencyRepo(store)
	ctx := context.Background()

	rec := &domain.IdempotencyRecord{
		Key: "sub_M1:B1", BusinessKey: "M1:B1", Outcome: "created",
	}
	require.NoError(t, repo.Save(ctx, rec))

	found, ok, err := repo.Get(ctx, "sub_M1:B1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "M1:B1", found.BusinessKey)

	rec2, err := repo.FindByBusinessKey(ctx, "M1:B1")
	require.NoError(t, err)
	require.NotNil(t, rec2)
	assert.Equal(t, "sub_M1:B1", rec2.Key)
}

func TestRepo_DeadLetterCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewDeadLetterRepo(store)
	ctx := context.Background()

	dl := &domain.DeadLetter{
		ID: "dl_1", EntityID: "dsp_1", EntityType: "dispatch_task",
		Reason: "max retries exceeded", LastError: "timeout",
		Attempts: 5, Resolved: false,
	}
	require.NoError(t, repo.Save(ctx, dl))

	found, ok, err := repo.Get(ctx, "dl_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, found.Resolved)

	dls, err := repo.FindByEntity(ctx, "dispatch_task", "dsp_1")
	require.NoError(t, err)
	assert.Len(t, dls, 1)

	dls, err = repo.ListUnresolved(ctx)
	require.NoError(t, err)
	assert.Len(t, dls, 1)
}

func TestRepo_RuleCRUD(t *testing.T) {
	store := newTestStore(t)
	repo := NewRuleRepo(store)
	ctx := context.Background()

	rule := &domain.Rule{
		ID: "rule_1", StandardCode: "GB-9706", Name: "Test Rule",
		Expression: `submission.risk_level >= 2`, Priority: 1, Status: domain.RuleDraft,
	}
	require.NoError(t, repo.SaveRule(ctx, rule))

	found, ok, err := repo.GetRule(ctx, "rule_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Test Rule", found.Name)

	rules, err := repo.FindByStandard(ctx, "GB-9706")
	require.NoError(t, err)
	assert.Len(t, rules, 1)

	rules, err = repo.FindByStatus(ctx, domain.RuleDraft)
	require.NoError(t, err)
	assert.Len(t, rules, 1)

	version := &domain.RuleVersion{
		ID: "ver_1", RuleID: "rule_1", StandardCode: "GB-9706",
		Version: 1, Expression: `submission.risk_level >= 2`, Priority: 1,
		Status: domain.RuleActive,
	}
	require.NoError(t, repo.SaveVersion(ctx, version))

	versions, err := repo.FindVersionsByRule(ctx, "rule_1")
	require.NoError(t, err)
	assert.Len(t, versions, 1)

	trial := &domain.RuleTrial{
		ID: "trial_1", RuleID: "rule_1", Expression: `submission.risk_level >= 2`,
		Matched: true, Status: domain.TrialCompleted,
	}
	require.NoError(t, repo.SaveTrial(ctx, trial))

	trials, err := repo.FindTrialsByRule(ctx, "rule_1")
	require.NoError(t, err)
	assert.Len(t, trials, 1)
}

func TestRepo_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := NewStore(kv)
	repo := NewSubmissionRepo(store)

	sub := &domain.Submission{ID: "sub_1", EnterpriseID: "ent", ModelNo: "M1", BatchNo: "B1", Category: domain.CategoryExoskeleton, StandardCodes: []string{"S"}, Status: domain.SubmissionPending}
	require.NoError(t, repo.Save(ctx, sub))
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := NewStore(kv2)
	repo2 := NewSubmissionRepo(store2)

	found, ok, err := repo2.Get(ctx, "sub_1")
	require.NoError(t, err)
	assert.True(t, ok, "should survive restart")
	assert.Equal(t, "M1", found.ModelNo)

	subs, err := repo2.FindByModelBatch(ctx, "M1", "B1")
	require.NoError(t, err)
	assert.Len(t, subs, 1, "secondary index should survive restart")
}
