package repository

import (
	"context"

	"ruralhealth/internal/domain"
)

type CallbackRepo struct {
	store *Store
}

func NewCallbackRepo(store *Store) *CallbackRepo {
	return &CallbackRepo{store: store}
}

func (r *CallbackRepo) Save(ctx context.Context, c *domain.Callback) error {
	indexes := []IndexEntry{
		{Name: "submission", Key: c.SubmissionID},
		{Name: "status", Key: string(c.Status)},
	}
	return r.store.PutJSON(ctx, BucketCallback, c.ID, c, indexes)
}

func (r *CallbackRepo) Get(ctx context.Context, id string) (*domain.Callback, bool, error) {
	var c domain.Callback
	found, err := r.store.GetJSON(ctx, BucketCallback, id, &c)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &c, true, nil
}

func (r *CallbackRepo) FindBySubmission(ctx context.Context, submissionID string) ([]*domain.Callback, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketCallback, "submission", submissionID)
	if err != nil {
		return nil, err
	}
	var results []*domain.Callback
	for _, key := range keys {
		c, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, c)
		}
	}
	return results, nil
}

func (r *CallbackRepo) FindByStatus(ctx context.Context, status domain.CallbackStatus) ([]*domain.Callback, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketCallback, "status", string(status))
	if err != nil {
		return nil, err
	}
	var results []*domain.Callback
	for _, key := range keys {
		c, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, c)
		}
	}
	return results, nil
}
