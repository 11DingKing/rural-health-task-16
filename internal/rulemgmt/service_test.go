package rulemgmt

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
	"ruralhealth/internal/ruleengine"
)

type testClock struct{ *clock.Mock }

func (t testClock) Now() time.Time                         { return t.Mock.Now() }
func (t testClock) NewTimer(d time.Duration) *time.Timer   { return time.NewTimer(d) }
func (t testClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
func (t testClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t testClock) Since(t2 time.Time) time.Duration       { return t.Mock.Now().Sub(t2) }

func newTestRuleService(t *testing.T) (*Service, *persistence.KVStore) {
	t.Helper()
	dir := t.TempDir()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(context.Background()))
	t.Cleanup(func() { kv.Close() })
	store := repository.NewStore(kv)
	ruleRepo := repository.NewRuleRepo(store)
	clk := testClock{clock.NewMock()}
	svc := NewService(ruleRepo, logging.NewNop(), clk)
	return svc, kv
}

func TestRule_CreateStandard(t *testing.T) {
	svc, _ := newTestRuleService(t)
	std := &domain.Standard{Code: "GB-9706", Name: "Medical Electrical Equipment", Category: domain.CategoryExoskeleton}
	result, err := svc.CreateStandard(context.Background(), std)
	require.NoError(t, err)
	assert.Equal(t, "GB-9706", result.Code)
	assert.True(t, result.IsActive)
}

func TestRule_CreateStandardDuplicate(t *testing.T) {
	svc, _ := newTestRuleService(t)
	std := &domain.Standard{Code: "GB-9706", Name: "Medical", Category: domain.CategoryExoskeleton}
	_, err := svc.CreateStandard(context.Background(), std)
	require.NoError(t, err)
	_, err = svc.CreateStandard(context.Background(), std)
	assert.Error(t, err)
}

func TestRule_ListStandards(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	_, err := svc.CreateStandard(ctx, &domain.Standard{Code: "S1", Name: "Std1", Category: domain.CategoryExoskeleton})
	require.NoError(t, err)
	_, err = svc.CreateStandard(ctx, &domain.Standard{Code: "S2", Name: "Std2", Category: domain.CategoryExoskeleton})
	require.NoError(t, err)
	standards, err := svc.ListStandards(ctx)
	require.NoError(t, err)
	assert.Len(t, standards, 2)
}

func TestRule_CreateRule(t *testing.T) {
	svc, _ := newTestRuleService(t)
	rule := &domain.Rule{StandardCode: "GB-9706", Name: "High Risk Rule", Expression: `submission.risk_level >= 2`, Priority: 1}
	result, err := svc.CreateRule(context.Background(), rule)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, domain.RuleDraft, result.Status)
}

func TestRule_CreateRuleInvalidExpression(t *testing.T) {
	svc, _ := newTestRuleService(t)
	rule := &domain.Rule{StandardCode: "GB-9706", Name: "Bad", Expression: `submission. >>> bad`, Priority: 1}
	_, err := svc.CreateRule(context.Background(), rule)
	assert.Error(t, err)
}

func TestRule_TrialCalculation(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2 && submission.category == "exoskeleton"`, Priority: 1})
	require.NoError(t, err)

	contextJSON := `{"submission": {"risk_level": 3, "category": "exoskeleton"}}`
	trial, err := svc.StartTrial(ctx, rule.ID, contextJSON)
	require.NoError(t, err)
	assert.True(t, trial.Matched)
	assert.Equal(t, domain.TrialCompleted, trial.Status)
	assert.NotEmpty(t, trial.Evidence)
}

func TestRule_TrialNoMatch(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 5`, Priority: 1})
	require.NoError(t, err)

	contextJSON := `{"submission": {"risk_level": 1}}`
	trial, err := svc.StartTrial(ctx, rule.ID, contextJSON)
	require.NoError(t, err)
	assert.False(t, trial.Matched)
}

func TestRule_ApproveTrial(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)

	contextJSON := `{"submission": {"risk_level": 3}}`
	trial, err := svc.StartTrial(ctx, rule.ID, contextJSON)
	require.NoError(t, err)

	approved, err := svc.ApproveTrial(ctx, trial.ID, "auditor_1")
	require.NoError(t, err)
	assert.Equal(t, domain.RuleActive, approved.Status)

	versions, err := svc.FindVersionsByRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1)
	assert.Equal(t, domain.RuleActive, versions[0].Status)
}

func TestRule_RejectTrial(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)

	contextJSON := `{"submission": {"risk_level": 3}}`
	trial, err := svc.StartTrial(ctx, rule.ID, contextJSON)
	require.NoError(t, err)

	rejected, err := svc.RejectTrial(ctx, trial.ID, "auditor_1", "evidence insufficient")
	require.NoError(t, err)
	assert.Equal(t, domain.TrialRejected, rejected.Status)

	ruleUpdated, err := svc.GetRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.RuleDraft, ruleUpdated.Status, "rule should return to draft after rejection")
}

func TestRule_UpdateRuleExpression(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)

	updated, err := svc.UpdateRuleExpression(ctx, rule.ID, `submission.risk_level >= 3`)
	require.NoError(t, err)
	assert.Equal(t, `submission.risk_level >= 3`, updated.Expression)
}

func TestRule_UpdateRuleExpressionInvalid(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)

	_, err = svc.UpdateRuleExpression(ctx, rule.ID, `bad expression >>>`)
	assert.Error(t, err)
}

func TestRule_ListRules(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	_, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "S1", Name: "R1", Expression: `submission.risk_level >= 1`, Priority: 1})
	require.NoError(t, err)
	_, err = svc.CreateRule(ctx, &domain.Rule{StandardCode: "S2", Name: "R2", Expression: `submission.risk_level >= 2`, Priority: 2})
	require.NoError(t, err)

	rules, err := svc.ListRules(ctx, "")
	require.NoError(t, err)
	assert.Len(t, rules, 2)

	rules, err = svc.ListRules(ctx, "S1")
	require.NoError(t, err)
	assert.Len(t, rules, 1)
}

func TestRule_EvaluateRule(t *testing.T) {
	svc, _ := newTestRuleService(t)
	ctx := context.Background()
	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)

	contextJSON := `{"submission": {"risk_level": 3, "category": "exoskeleton"}}`
	trial, err := svc.StartTrial(ctx, rule.ID, contextJSON)
	require.NoError(t, err)
	_, err = svc.ApproveTrial(ctx, trial.ID, "auditor")
	require.NoError(t, err)

	ec := ruleengine.NewEvalContext()
	ec.Set("submission", "risk_level", 3.0)
	ec.Set("submission", "category", "exoskeleton")
	matched, evidence, err := svc.EvaluateRule(ctx, rule.ID, ec)
	require.NoError(t, err)
	assert.True(t, matched)
	assert.NotEmpty(t, evidence)
}

func TestRule_PersistRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	kv := persistence.NewKVStore(dir)
	require.NoError(t, kv.Open(ctx))
	store := repository.NewStore(kv)
	ruleRepo := repository.NewRuleRepo(store)
	clk := testClock{clock.NewMock()}
	svc := NewService(ruleRepo, logging.NewNop(), clk)

	rule, err := svc.CreateRule(ctx, &domain.Rule{StandardCode: "GB-9706", Name: "R", Expression: `submission.risk_level >= 2`, Priority: 1})
	require.NoError(t, err)
	require.NoError(t, kv.Close())

	kv2 := persistence.NewKVStore(dir)
	require.NoError(t, kv2.Open(ctx))
	defer kv2.Close()
	store2 := repository.NewStore(kv2)
	ruleRepo2 := repository.NewRuleRepo(store2)
	svc2 := NewService(ruleRepo2, logging.NewNop(), clk)

	recovered, err := svc2.GetRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, rule.ID, recovered.ID)
	assert.Equal(t, rule.Expression, recovered.Expression)
}

func TestRule_InvalidTransitionReject(t *testing.T) {
	rsm := RuleStateMachine{}
	err := rsm.Transition(domain.RuleDraft, domain.RuleActive)
	assert.Error(t, err, "draft -> active is invalid")
	err = rsm.Transition(domain.RuleRetired, domain.RuleActive)
	assert.Error(t, err, "retired -> active is invalid")
}
