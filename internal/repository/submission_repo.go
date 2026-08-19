package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"ruralhealth/internal/domain"
)

type SubmissionRepo struct {
	store *Store
}

func NewSubmissionRepo(store *Store) *SubmissionRepo {
	return &SubmissionRepo{store: store}
}

func (r *SubmissionRepo) Save(ctx context.Context, sub *domain.Submission) error {
	indexes := []IndexEntry{
		{Name: "model_batch", Key: sub.ModelNo + ":" + sub.BatchNo},
		{Name: "enterprise", Key: sub.EnterpriseID},
		{Name: "status", Key: string(sub.Status)},
	}
	return r.store.PutJSON(ctx, BucketSubmission, sub.ID, sub, indexes)
}

func (r *SubmissionRepo) Get(ctx context.Context, id string) (*domain.Submission, bool, error) {
	var sub domain.Submission
	found, err := r.store.GetJSON(ctx, BucketSubmission, id, &sub)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &sub, true, nil
}

func (r *SubmissionRepo) FindByModelBatch(ctx context.Context, modelNo, batchNo string) ([]*domain.Submission, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketSubmission, "model_batch", modelNo+":"+batchNo)
	if err != nil {
		return nil, err
	}
	var results []*domain.Submission
	for _, key := range keys {
		sub, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, sub)
		}
	}
	return results, nil
}

func (r *SubmissionRepo) FindByEnterprise(ctx context.Context, enterpriseID string) ([]*domain.Submission, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketSubmission, "enterprise", enterpriseID)
	if err != nil {
		return nil, err
	}
	var results []*domain.Submission
	for _, key := range keys {
		sub, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, sub)
		}
	}
	return results, nil
}

func (r *SubmissionRepo) FindByStatus(ctx context.Context, status domain.SubmissionStatus) ([]*domain.Submission, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketSubmission, "status", string(status))
	if err != nil {
		return nil, err
	}
	var results []*domain.Submission
	for _, key := range keys {
		sub, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, sub)
		}
	}
	return results, nil
}

func (r *SubmissionRepo) ListPaged(ctx context.Context, params domain.PaginationParams, statusFilter, enterpriseFilter string) ([]*domain.Submission, int, error) {
	var all []*domain.Submission
	if statusFilter != "" {
		subs, err := r.FindByStatus(ctx, domain.SubmissionStatus(statusFilter))
		if err != nil {
			return nil, 0, err
		}
		all = subs
	} else {
		err := r.store.ScanPrefix(ctx, BucketSubmission, "", func(key string, data []byte) bool {
			var sub domain.Submission
			if err := json.Unmarshal(data, &sub); err != nil {
				return false
			}
			all = append(all, &sub)
			return true
		})
		if err != nil {
			return nil, 0, err
		}
	}
	if enterpriseFilter != "" {
		var filtered []*domain.Submission
		for _, s := range all {
			if s.EnterpriseID == enterpriseFilter {
				filtered = append(filtered, s)
			}
		}
		all = filtered
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.Submission{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func (r *SubmissionRepo) Delete(ctx context.Context, id string) error {
	return r.store.Delete(ctx, BucketSubmission, id)
}

func (r *SubmissionRepo) Count(ctx context.Context) (int, error) {
	return r.store.Count(ctx, BucketSubmission)
}

func init() {
	_ = fmt.Sprintf
}
