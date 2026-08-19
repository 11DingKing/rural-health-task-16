package repository

import (
	"context"
	"encoding/json"

	"ruralhealth/internal/domain"
)

type DeadLetterRepo struct {
	store *Store
}

func NewDeadLetterRepo(store *Store) *DeadLetterRepo {
	return &DeadLetterRepo{store: store}
}

func (r *DeadLetterRepo) Save(ctx context.Context, dl *domain.DeadLetter) error {
	indexes := []IndexEntry{
		{Name: "entity", Key: dl.EntityType + ":" + dl.EntityID},
		{Name: "resolved", Key: boolStr(dl.Resolved)},
	}
	return r.store.PutJSON(ctx, BucketDeadLetter, dl.ID, dl, indexes)
}

func (r *DeadLetterRepo) Get(ctx context.Context, id string) (*domain.DeadLetter, bool, error) {
	var dl domain.DeadLetter
	found, err := r.store.GetJSON(ctx, BucketDeadLetter, id, &dl)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return &dl, true, nil
}

func (r *DeadLetterRepo) FindByEntity(ctx context.Context, entityType, entityID string) ([]*domain.DeadLetter, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDeadLetter, "entity", entityType+":"+entityID)
	if err != nil {
		return nil, err
	}
	var results []*domain.DeadLetter
	for _, key := range keys {
		dl, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, dl)
		}
	}
	return results, nil
}

func (r *DeadLetterRepo) ListUnresolved(ctx context.Context) ([]*domain.DeadLetter, error) {
	keys, err := r.store.LookupByIndex(ctx, BucketDeadLetter, "resolved", "false")
	if err != nil {
		return nil, err
	}
	var results []*domain.DeadLetter
	for _, key := range keys {
		dl, found, err := r.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, dl)
		}
	}
	return results, nil
}

func (r *DeadLetterRepo) ListPaged(ctx context.Context, params domain.PaginationParams) ([]*domain.DeadLetter, int, error) {
	var all []*domain.DeadLetter
	err := r.store.ScanPrefix(ctx, BucketDeadLetter, "", func(key string, data []byte) bool {
		var dl domain.DeadLetter
		if err := json.Unmarshal(data, &dl); err != nil {
			return false
		}
		all = append(all, &dl)
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	params = params.Normalize()
	start := params.Offset()
	if start >= total {
		return []*domain.DeadLetter{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
