package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type IdempotencyRepo struct {
	store *Store
}

func NewIdempotencyRepo(store *Store) *IdempotencyRepo {
	return &IdempotencyRepo{store: store}
}

func (r *IdempotencyRepo) Save(ctx context.Context, rec *domain.IdempotencyRecord) error {
	indexes := []IndexEntry{
		{Name: "business", Key: rec.BusinessKey},
	}
	return r.store.PutJSON(ctx, BucketIdempotency, rec.Key, rec, indexes)
}

func (r *IdempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, bool, error) {
	var rec domain.IdempotencyRecord
	found, err := r.store.GetJSON(ctx, BucketIdempotency, key, &rec)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (r *IdempotencyRepo) FindByBusinessKey(ctx context.Context, businessKey string) (*domain.IdempotencyRecord, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketIdempotency, "business", businessKey)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	rec, _, err := r.Get(ctx, keys[0])
	return rec, err
}

func (r *IdempotencyRepo) Delete(ctx context.Context, key string) error {
	return r.store.Delete(ctx, BucketIdempotency, key)
}

func (r *IdempotencyRepo) ListPaged(ctx context.Context, params domain.PaginationParams) ([]*domain.IdempotencyRecord, int, error) {
	var all []*domain.IdempotencyRecord
	err := r.store.ScanPrefix(ctx, BucketIdempotency, "", func(key string, data []byte) bool {
		var rec domain.IdempotencyRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return false
		}
		all = append(all, &rec)
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.IdempotencyRecord{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}
