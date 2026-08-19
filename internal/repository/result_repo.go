package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type ResultRepo struct {
	store *Store
}

func NewResultRepo(store *Store) *ResultRepo {
	return &ResultRepo{store: store}
}

func (r *ResultRepo) Save(ctx context.Context, res *domain.TestResult) error {
	indexes := []IndexEntry{
		{Name: "dispatch", Key: res.DispatchID},
		{Name: "submission", Key: res.SubmissionID},
		{Name: "model", Key: res.ModelNo},
		{Name: "standard", Key: res.StandardCode},
	}
	return r.store.PutJSON(ctx, BucketResult, res.ID, res, indexes)
}

func (r *ResultRepo) Get(ctx context.Context, id string) (*domain.TestResult, bool, error) {
	var res domain.TestResult
	found, err := r.store.GetJSON(ctx, BucketResult, id, &res)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &res, true, nil
}

func (r *ResultRepo) FindByDispatch(ctx context.Context, dispatchID string) ([]*domain.TestResult, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketResult, "dispatch", dispatchID)
	if err != nil {
		return nil, err
	}
	var results []*domain.TestResult
	for _, key := range keys {
		res, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, res)
		}
	}
	return results, nil
}

func (r *ResultRepo) FindBySubmission(ctx context.Context, submissionID string) ([]*domain.TestResult, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketResult, "submission", submissionID)
	if err != nil {
		return nil, err
	}
	var results []*domain.TestResult
	for _, key := range keys {
		res, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, res)
		}
	}
	return results, nil
}

func (r *ResultRepo) FindByModel(ctx context.Context, modelNo string) ([]*domain.TestResult, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketResult, "model", modelNo)
	if err != nil {
		return nil, err
	}
	var results []*domain.TestResult
	for _, key := range keys {
		res, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, res)
		}
	}
	return results, nil
}

func (r *ResultRepo) ListPaged(ctx context.Context, params domain.PaginationParams) ([]*domain.TestResult, int, error) {
	var all []*domain.TestResult
	err := r.store.ScanPrefix(ctx, BucketResult, "", func(key string, data []byte) bool {
		var res domain.TestResult
		if err := json.Unmarshal(data, &res); err != nil {
			return false
		}
		all = append(all, &res)
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.TestResult{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}
