package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"ruralhealth/internal/persistence"
)

// Bucket names for each entity type.
const (
	BucketSubmission  = "submissions"
	BucketStandard    = "standards"
	BucketRule        = "rules"
	BucketRuleVersion = "rule_versions"
	BucketRuleTrial   = "rule_trials"
	BucketAgency      = "agencies"
	BucketDispatch    = "dispatch_tasks"
	BucketResult      = "test_results"
	BucketSummary     = "summaries"
	BucketCallback    = "callbacks"
	BucketIdempotency = "idempotency"
	BucketDeadLetter  = "dead_letters"
)

// Store wraps the persistence KV store with JSON encoding helpers.
type Store struct {
	kv *persistence.KVStore
}

func NewStore(kv *persistence.KVStore) *Store {
	return &Store{kv: kv}
}

func (s *Store) KV() *persistence.KVStore { return s.kv }

// PutJSON marshals a value to JSON and stores it with optional indexes.
func (s *Store) PutJSON(ctx context.Context, bucket, key string, val any, indexes []IndexEntry) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", bucket, key, err)
	}
	tx := s.kv.BeginTx()
	if err := tx.Put(bucket, key, data, "", ""); err != nil {
		return err
	}
	for _, idx := range indexes {
		tx.AddIndex(bucket, idx.Name, idx.Key, key)
	}
	return tx.Commit(ctx)
}

// PutJSONWithPrimary stores a value with a single primary index entry.
func (s *Store) PutJSONWithPrimary(ctx context.Context, bucket, key string, val any, indexName, indexKey string) error {
	return s.PutJSON(ctx, bucket, key, val, []IndexEntry{{Name: indexName, Key: indexKey}})
}

// GetJSON retrieves a value by key and unmarshals it.
func (s *Store) GetJSON(ctx context.Context, bucket, key string, target any) (bool, error) {
	data, ok, err := s.kv.Get(ctx, bucket, key)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, fmt.Errorf("unmarshal %s/%s: %w", bucket, key, err)
	}
	return true, nil
}

// Delete removes a key and all its index entries.
func (s *Store) Delete(ctx context.Context, bucket, key string) error {
	return s.kv.Delete(ctx, bucket, key)
}

// LookupByIndex returns all primary keys matching a secondary index.
func (s *Store) LookupByIndex(ctx context.Context, bucket, indexName, indexKey string) ([]string, error) {
	return s.kv.LookupByIndex(ctx, bucket, indexName, indexKey)
}

// ScanPrefix iterates keys with a prefix, returning decoded values.
func (s *Store) ScanPrefix(ctx context.Context, bucket, prefix string, decode func(key string, data []byte) bool) error {
	return s.kv.ScanPrefix(ctx, bucket, prefix, func(key string, value []byte) bool {
		return decode(key, value)
	})
}

// ListPaged scans all keys in a bucket, decodes, sorts, and paginates.
func (s *Store) ListPaged(ctx context.Context, bucket string, page, pageSize int, less func(a, b string) bool, decode func(key string, data []byte) (any, error)) ([]any, int, error) {
	var keys []string
	if err := s.kv.ScanPrefix(ctx, bucket, "", func(key string, _ []byte) bool {
		keys = append(keys, key)
		return true
	}); err != nil {
		return nil, 0, err
	}
	if less != nil {
		sort.Slice(keys, func(i, j int) bool {
			return less(keys[i], keys[j])
		})
	} else {
		sort.Strings(keys)
	}
	total := len(keys)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []any{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	result := make([]any, 0, end-start)
	for _, key := range keys[start:end] {
		data, ok, err := s.kv.Get(ctx, bucket, key)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			continue
		}
		val, err := decode(key, data)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, val)
	}
	return result, total, nil
}

type IndexEntry struct {
	Name string
	Key  string
}

// Count returns the number of records in a bucket.
func (s *Store) Count(ctx context.Context, bucket string) (int, error) {
	return s.kv.Count(ctx, bucket)
}
