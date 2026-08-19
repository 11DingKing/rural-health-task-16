package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type RuleRepo struct {
	store *Store
}

func NewRuleRepo(store *Store) *RuleRepo {
	return &RuleRepo{store: store}
}

func (r *RuleRepo) SaveRule(ctx context.Context, rule *domain.Rule) error {
	indexes := []IndexEntry{
		{Name: "standard", Key: rule.StandardCode},
		{Name: "status", Key: string(rule.Status)},
	}
	return r.store.PutJSON(ctx, BucketRule, rule.ID, rule, indexes)
}

func (r *RuleRepo) GetRule(ctx context.Context, id string) (*domain.Rule, bool, error) {
	var rule domain.Rule
	found, err := r.store.GetJSON(ctx, BucketRule, id, &rule)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &rule, true, nil
}

func (r *RuleRepo) FindByStandard(ctx context.Context, standardCode string) ([]*domain.Rule, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketRule, "standard", standardCode)
	if err != nil {
		return nil, err
	}
	var results []*domain.Rule
	for _, key := range keys {
		rule, found, err := r.GetRule(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, rule)
		}
	}
	return results, nil
}

func (r *RuleRepo) FindByStatus(ctx context.Context, status domain.RuleStatus) ([]*domain.Rule, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketRule, "status", string(status))
	if err != nil {
		return nil, err
	}
	var results []*domain.Rule
	for _, key := range keys {
		rule, found, err := r.GetRule(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, rule)
		}
	}
	return results, nil
}

func (r *RuleRepo) SaveVersion(ctx context.Context, v *domain.RuleVersion) error {
	indexes := []IndexEntry{
		{Name: "rule", Key: v.RuleID},
		{Name: "standard", Key: v.StandardCode},
		{Name: "status", Key: string(v.Status)},
	}
	return r.store.PutJSON(ctx, BucketRuleVersion, v.ID, v, indexes)
}

func (r *RuleRepo) GetVersion(ctx context.Context, id string) (*domain.RuleVersion, bool, error) {
	var v domain.RuleVersion
	found, err := r.store.GetJSON(ctx, BucketRuleVersion, id, &v)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &v, true, nil
}

func (r *RuleRepo) FindVersionsByRule(ctx context.Context, ruleID string) ([]*domain.RuleVersion, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketRuleVersion, "rule", ruleID)
	if err != nil {
		return nil, err
	}
	var results []*domain.RuleVersion
	for _, key := range keys {
		v, found, err := r.GetVersion(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, v)
		}
	}
	return results, nil
}

func (r *RuleRepo) SaveTrial(ctx context.Context, t *domain.RuleTrial) error {
	indexes := []IndexEntry{
		{Name: "rule", Key: t.RuleID},
		{Name: "status", Key: string(t.Status)},
	}
	return r.store.PutJSON(ctx, BucketRuleTrial, t.ID, t, indexes)
}

func (r *RuleRepo) GetTrial(ctx context.Context, id string) (*domain.RuleTrial, bool, error) {
	var t domain.RuleTrial
	found, err := r.store.GetJSON(ctx, BucketRuleTrial, id, &t)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &t, true, nil
}

func (r *RuleRepo) FindTrialsByRule(ctx context.Context, ruleID string) ([]*domain.RuleTrial, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketRuleTrial, "rule", ruleID)
	if err != nil {
		return nil, err
	}
	var results []*domain.RuleTrial
	for _, key := range keys {
		t, found, err := r.GetTrial(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, t)
		}
	}
	return results, nil
}

func (r *RuleRepo) SaveStandard(ctx context.Context, s *domain.Standard) error {
	return r.store.PutJSONWithPrimary(ctx, BucketStandard, s.Code, s, "category", string(s.Category))
}

func (r *RuleRepo) GetStandard(ctx context.Context, code string) (*domain.Standard, bool, error) {
	var s domain.Standard
	found, err := r.store.GetJSON(ctx, BucketStandard, code, &s)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &s, true, nil
}

func (r *RuleRepo) ListStandards(ctx context.Context) ([]*domain.Standard, error) {
	var results []*domain.Standard
	err := r.store.ScanPrefix(ctx, BucketStandard, "", func(key string, data []byte) bool {
		var s domain.Standard
		if err := json.Unmarshal(data, &s); err != nil {
			return false
		}
		results = append(results, &s)
		return true
	})
	return results, err
}
