package repository

import (
	"context"
	"encoding/json"
	"sort"

	"ruralhealth/internal/domain"
)

type DispatchRepo struct {
	store *Store
}

func NewDispatchRepo(store *Store) *DispatchRepo {
	return &DispatchRepo{store: store}
}

func (r *DispatchRepo) Save(ctx context.Context, t *domain.DispatchTask) error {
	indexes := []IndexEntry{
		{Name: "submission", Key: t.SubmissionID},
		{Name: "agency", Key: t.AgencyID},
		{Name: "status", Key: string(t.Status)},
		{Name: "model_batch", Key: t.ModelNo + ":" + t.BatchNo},
	}
	return r.store.PutJSON(ctx, BucketDispatch, t.ID, t, indexes)
}

func (r *DispatchRepo) Get(ctx context.Context, id string) (*domain.DispatchTask, bool, error) {
	var t domain.DispatchTask
	found, err := r.store.GetJSON(ctx, BucketDispatch, id, &t)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &t, true, nil
}

func (r *DispatchRepo) FindBySubmission(ctx context.Context, submissionID string) ([]*domain.DispatchTask, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDispatch, "submission", submissionID)
	if err != nil {
		return nil, err
	}
	var results []*domain.DispatchTask
	for _, key := range keys {
		t, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, t)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].AssignedAt.Before(results[j].AssignedAt)
	})
	return results, nil
}

func (r *DispatchRepo) FindByAgency(ctx context.Context, agencyID string) ([]*domain.DispatchTask, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDispatch, "agency", agencyID)
	if err != nil {
		return nil, err
	}
	var results []*domain.DispatchTask
	for _, key := range keys {
		t, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, t)
		}
	}
	return results, nil
}

func (r *DispatchRepo) FindByStatus(ctx context.Context, status domain.DispatchStatus) ([]*domain.DispatchTask, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDispatch, "status", string(status))
	if err != nil {
		return nil, err
	}
	var results []*domain.DispatchTask
	for _, key := range keys {
		t, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, t)
		}
	}
	return results, nil
}

func (r *DispatchRepo) FindByModelBatch(ctx context.Context, modelNo, batchNo string) ([]*domain.DispatchTask, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDispatch, "model_batch", modelNo+":"+batchNo)
	if err != nil {
		return nil, err
	}
	var results []*domain.DispatchTask
	for _, key := range keys {
		t, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, t)
		}
	}
	return results, nil
}

func (r *DispatchRepo) ListPaged(ctx context.Context, params domain.PaginationParams, agencyFilter, statusFilter string) ([]*domain.DispatchTask, int, error) {
	var all []*domain.DispatchTask
	if statusFilter != "" {
		tasks, err := r.FindByStatus(ctx, domain.DispatchStatus(statusFilter))
		if err != nil {
			return nil, 0, err
		}
		all = tasks
	} else {
		err := r.store.ScanPrefix(ctx, BucketDispatch, "", func(key string, data []byte) bool {
			var t domain.DispatchTask
			if err := json.Unmarshal(data, &t); err != nil {
				return false
			}
			all = append(all, &t)
			return true
		})
		if err != nil {
			return nil, 0, err
		}
	}
	if agencyFilter != "" {
		var filtered []*domain.DispatchTask
		for _, t := range all {
			if t.AgencyID == agencyFilter {
				filtered = append(filtered, t)
			}
		}
		all = filtered
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.DispatchTask{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func (r *DispatchRepo) Delete(ctx context.Context, id string) error {
	return r.store.Delete(ctx, BucketDispatch, id)
}
