package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type SummaryRepo struct {
	store *Store
}

func NewSummaryRepo(store *Store) *SummaryRepo {
	return &SummaryRepo{store: store}
}

func (r *SummaryRepo) Save(ctx context.Context, s *domain.Summary) error {
	indexes := []IndexEntry{
		{Name: "submission", Key: s.SubmissionID},
		{Name: "model", Key: s.ModelNo},
		{Name: "status", Key: string(s.Status)},
	}
	return r.store.PutJSON(ctx, BucketSummary, s.ID, s, indexes)
}

func (r *SummaryRepo) Get(ctx context.Context, id string) (*domain.Summary, bool, error) {
	var s domain.Summary
	found, err := r.store.GetJSON(ctx, BucketSummary, id, &s)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &s, true, nil
}

func (r *SummaryRepo) FindBySubmission(ctx context.Context, submissionID string) (*domain.Summary, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketSummary, "submission", submissionID)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	s, _, err := r.Get(ctx, keys[0])
	return s, err
}

func (r *SummaryRepo) FindByModel(ctx context.Context, modelNo string) ([]*domain.Summary, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketSummary, "model", modelNo)
	if err != nil {
		return nil, err
	}
	var results []*domain.Summary
	for _, key := range keys {
		s, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, s)
		}
	}
	return results, nil
}

func (r *SummaryRepo) ListPaged(ctx context.Context, params domain.PaginationParams) ([]*domain.Summary, int, error) {
	var all []*domain.Summary
	err := r.store.ScanPrefix(ctx, BucketSummary, "", func(key string, data []byte) bool {
		var s domain.Summary
		if err := json.Unmarshal(data, &s); err != nil {
			return false
		}
		all = append(all, &s)
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.Summary{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}
