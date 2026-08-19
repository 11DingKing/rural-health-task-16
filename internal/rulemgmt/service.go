package rulemgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
	"ruralhealth/internal/ruleengine"
)

type Service struct {
	ruleRepo *repository.RuleRepo
	engine   *ruleengine.Engine
	logger   logging.Logger
	clk      clock.Clock
	sm       RuleStateMachine
}

type RuleStateMachine struct{}

func (rsm RuleStateMachine) Transition(current, target domain.RuleStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func (rsm RuleStateMachine) TrialTransition(current, target domain.TrialStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func NewService(ruleRepo *repository.RuleRepo, logger logging.Logger, clk clock.Clock) *Service {
	return &Service{
		ruleRepo: ruleRepo,
		engine:   ruleengine.NewEngine(),
		logger:   logger,
		clk:      clk,
		sm:       RuleStateMachine{},
	}
}

func (s *Service) CreateStandard(ctx context.Context, std *domain.Standard) (*domain.Standard, error) {
	existing, found, err := s.ruleRepo.GetStandard(ctx, std.Code)
	if err != nil {
		return nil, err
	}
	if found {
		_ = existing
		return nil, fmt.Errorf("%w: standard %s already exists", errorsx.ErrAlreadyExists, std.Code)
	}
	now := s.clk.Now()
	std.Audit = domain.AuditMeta{
		CreatedBy: "auditor", CreatedAt: now, UpdatedBy: "auditor", UpdatedAt: now,
	}
	std.IsActive = true
	if err := s.ruleRepo.SaveStandard(ctx, std); err != nil {
		return nil, err
	}
	return std, nil
}

func (s *Service) ListStandards(ctx context.Context) ([]*domain.Standard, error) {
	return s.ruleRepo.ListStandards(ctx)
}

func (s *Service) CreateRule(ctx context.Context, rule *domain.Rule) (*domain.Rule, error) {
	if _, err := s.engine.Compile(rule.Expression); err != nil {
		return nil, fmt.Errorf("%w: %s", errorsx.ErrValidation, err)
	}
	now := s.clk.Now()
	rule.ID = "rule_" + uuid.NewString()
	rule.Status = domain.RuleDraft
	rule.Audit = domain.AuditMeta{
		CreatedBy: "auditor", CreatedAt: now, UpdatedBy: "auditor", UpdatedAt: now,
	}
	if err := s.ruleRepo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) UpdateRuleExpression(ctx context.Context, id, expr string) (*domain.Rule, error) {
	rule, found, err := s.ruleRepo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if rule.Status != domain.RuleDraft && rule.Status != domain.RuleRetired {
		return nil, fmt.Errorf("%w: rule must be draft or retired", errorsx.ErrConflict)
	}
	if _, err := s.engine.Compile(expr); err != nil {
		return nil, fmt.Errorf("%w: %s", errorsx.ErrValidation, err)
	}
	rule.Expression = expr
	rule.Audit.UpdatedAt = s.clk.Now()
	if err := s.ruleRepo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) StartTrial(ctx context.Context, ruleID, contextJSON string) (*domain.RuleTrial, error) {
	rule, found, err := s.ruleRepo.GetRule(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	trialResult, err := s.engine.Trial(rule.Expression, contextJSON)
	now := s.clk.Now()
	trial := &domain.RuleTrial{
		ID: "trial_" + uuid.NewString(), RuleID: ruleID, Expression: rule.Expression,
		ContextJSON: contextJSON, Status: domain.TrialCreated,
		Audit: domain.AuditMeta{CreatedBy: "auditor", CreatedAt: now, UpdatedBy: "auditor", UpdatedAt: now},
	}
	if err != nil {
		trial.Status = domain.TrialFailed
		trial.ErrorMessage = err.Error()
	} else {
		trial.Matched = trialResult.Matched
		trial.Evidence = convertEvidence(trialResult.Evidence)
		if trialResult.Error != "" {
			trial.Status = domain.TrialFailed
			trial.ErrorMessage = trialResult.Error
		} else {
			trial.Status = domain.TrialCompleted
		}
	}
	if err := s.sm.Transition(rule.Status, domain.RuleTrialing); err != nil {
		return nil, err
	}
	rule.Status = domain.RuleTrialing
	rule.Audit.UpdatedAt = now
	if err := s.ruleRepo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	if err := s.ruleRepo.SaveTrial(ctx, trial); err != nil {
		return nil, err
	}
	return trial, nil
}

func convertEvidence(ev []ruleengine.Evidence) []domain.TrialEvidence {
	result := make([]domain.TrialEvidence, len(ev))
	for i, e := range ev {
		result[i] = domain.TrialEvidence{
			NodeID: e.NodeID, NodeType: e.NodeType, Source: e.Source,
			GotValue: e.GotValue, Expected: e.Expected, Passed: e.Passed,
		}
	}
	return result
}

func (s *Service) ApproveTrial(ctx context.Context, trialID, reviewedBy string) (*domain.Rule, error) {
	trial, found, err := s.ruleRepo.GetTrial(ctx, trialID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if err := s.sm.TrialTransition(trial.Status, domain.TrialApproved); err != nil {
		return nil, err
	}
	trial.Status = domain.TrialApproved
	now := s.clk.Now()
	trial.ReviewedBy = reviewedBy
	trial.ReviewedAt = &now
	trial.Audit.UpdatedAt = now
	if err := s.ruleRepo.SaveTrial(ctx, trial); err != nil {
		return nil, err
	}
	rule, found, err := s.ruleRepo.GetRule(ctx, trial.RuleID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if rule.Status == domain.RuleActive {
		oldVersions, err := s.ruleRepo.FindVersionsByRule(ctx, rule.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range oldVersions {
			if v.Status == domain.RuleActive {
				if err := s.sm.Transition(v.Status, domain.RuleSuperseded); err == nil {
					v.Status = domain.RuleSuperseded
					endTime := now
					v.EffectiveTo = &endTime
					_ = s.ruleRepo.SaveVersion(ctx, v)
				}
			}
		}
	}
	if err := s.sm.Transition(rule.Status, domain.RulePending); err != nil {
		return nil, err
	}
	rule.Status = domain.RulePending
	rule.Audit.UpdatedAt = now
	if err := s.ruleRepo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	if err := s.sm.Transition(rule.Status, domain.RuleActive); err != nil {
		return nil, err
	}
	rule.Status = domain.RuleActive
	rule.Audit.UpdatedAt = now
	if err := s.ruleRepo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	versions, err := s.ruleRepo.FindVersionsByRule(ctx, rule.ID)
	if err != nil {
		return nil, err
	}
	version := &domain.RuleVersion{
		ID: "ver_" + uuid.NewString(), RuleID: rule.ID, StandardCode: rule.StandardCode,
		Version: len(versions) + 1, Expression: rule.Expression, Priority: rule.Priority,
		EffectiveFrom: now, Status: domain.RuleActive,
		Audit: domain.AuditMeta{CreatedBy: reviewedBy, CreatedAt: now, UpdatedBy: reviewedBy, UpdatedAt: now},
	}
	if err := s.ruleRepo.SaveVersion(ctx, version); err != nil {
		return nil, err
	}
	s.logger.Info(ctx, "rule activated", "rule_id", rule.ID, "version", version.Version, "reviewed_by", reviewedBy)
	return rule, nil
}

func (s *Service) RejectTrial(ctx context.Context, trialID, reviewedBy, reason string) (*domain.RuleTrial, error) {
	trial, found, err := s.ruleRepo.GetTrial(ctx, trialID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if err := s.sm.TrialTransition(trial.Status, domain.TrialRejected); err != nil {
		return nil, err
	}
	trial.Status = domain.TrialRejected
	now := s.clk.Now()
	trial.ReviewedBy = reviewedBy
	trial.ReviewedAt = &now
	trial.ErrorMessage = reason
	trial.Audit.UpdatedAt = now
	if err := s.ruleRepo.SaveTrial(ctx, trial); err != nil {
		return nil, err
	}
	rule, found, err := s.ruleRepo.GetRule(ctx, trial.RuleID)
	if err != nil {
		return nil, err
	}
	if found && rule.Status == domain.RuleTrialing {
		if err := s.sm.Transition(rule.Status, domain.RuleDraft); err == nil {
			rule.Status = domain.RuleDraft
			rule.Audit.UpdatedAt = now
			_ = s.ruleRepo.SaveRule(ctx, rule)
		}
	}
	return trial, nil
}

func (s *Service) GetRule(ctx context.Context, id string) (*domain.Rule, error) {
	rule, found, err := s.ruleRepo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	return rule, nil
}

func (s *Service) ListRules(ctx context.Context, standardCode string) ([]*domain.Rule, error) {
	if standardCode != "" {
		return s.ruleRepo.FindByStandard(ctx, standardCode)
	}
	var rules []*domain.Rule
	for _, status := range []domain.RuleStatus{
		domain.RuleActive, domain.RuleDraft, domain.RuleTrialing,
		domain.RulePending, domain.RuleSuperseded, domain.RuleRetired,
	} {
		found, err := s.ruleRepo.FindByStatus(ctx, status)
		if err != nil {
			return nil, err
		}
		rules = append(rules, found...)
	}
	return rules, nil
}

func (s *Service) GetTrial(ctx context.Context, id string) (*domain.RuleTrial, error) {
	trial, found, err := s.ruleRepo.GetTrial(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	return trial, nil
}

func (s *Service) FindTrialsByRule(ctx context.Context, ruleID string) ([]*domain.RuleTrial, error) {
	return s.ruleRepo.FindTrialsByRule(ctx, ruleID)
}

func (s *Service) FindVersionsByRule(ctx context.Context, ruleID string) ([]*domain.RuleVersion, error) {
	return s.ruleRepo.FindVersionsByRule(ctx, ruleID)
}

func (s *Service) EvaluateRule(ctx context.Context, ruleID string, ec *ruleengine.EvalContext) (bool, []ruleengine.Evidence, error) {
	rule, found, err := s.ruleRepo.GetRule(ctx, ruleID)
	if err != nil {
		return false, nil, err
	}
	if !found {
		return false, nil, errorsx.ErrNotFound
	}
	if rule.Status != domain.RuleActive {
		return false, nil, fmt.Errorf("%w: rule %s is %s", errorsx.ErrRuleNotActive, ruleID, rule.Status)
	}
	result, evidence, err := s.engine.Evaluate(rule.Expression, ec)
	if err != nil {
		return false, nil, err
	}
	return ruleengine.IsTruthy(result), evidence, nil
}

func (s *Service) MarshalTrial(trial *domain.RuleTrial) (string, error) {
	data, err := json.Marshal(trial)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) TimeNow() time.Time { return s.clk.Now() }
