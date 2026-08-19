package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type AgencyRepo struct {
	store *Store
}

func NewAgencyRepo(store *Store) *AgencyRepo {
	return &AgencyRepo{store: store}
}

func (r *AgencyRepo) Save(ctx context.Context, a *domain.Agency) error {
	indexes := []IndexEntry{{Name: "code", Key: a.Code}}
	for _, std := range a.AccreditedStandards {
		indexes = append(indexes, IndexEntry{Name: "standard", Key: std})
	}
	return r.store.PutJSON(ctx, BucketAgency, a.ID, a, indexes)
}

func (r *AgencyRepo) Get(ctx context.Context, id string) (*domain.Agency, bool, error) {
	var a domain.Agency
	found, err := r.store.GetJSON(ctx, BucketAgency, id, &a)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &a, true, nil
}

func (r *AgencyRepo) FindByCode(ctx context.Context, code string) (*domain.Agency, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketAgency, "code", code)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	a, _, err := r.Get(ctx, keys[0])
	return a, err
}

func (r *AgencyRepo) FindByStandard(ctx context.Context, standardCode string) ([]*domain.Agency, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketAgency, "standard", standardCode)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var results []*domain.Agency
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		a, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, a)
		}
	}
	return results, nil
}

func (r *AgencyRepo) List(ctx context.Context) ([]*domain.Agency, error) {
	var results []*domain.Agency
	err := r.store.ScanPrefix(ctx, BucketAgency, "", func(key string, data []byte) bool {
		var a domain.Agency
		if err := json.Unmarshal(data, &a); err != nil {
			return false
		}
		results = append(results, &a)
		return true
	})
	return results, err
}

func (r *AgencyRepo) ListActive(ctx context.Context) ([]*domain.Agency, error) {
	all, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	var results []*domain.Agency
	for _, a := range all {
		if a.IsActive {
			results = append(results, a)
		}
	}
	return results, nil
}
