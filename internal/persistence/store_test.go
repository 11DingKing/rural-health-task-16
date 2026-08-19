package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *KVStore {
	t.Helper()
	dir := t.TempDir()
	s := NewKVStore(dir)
	require.NoError(t, s.Open(context.Background()))
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_PutAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.Put(ctx, "submissions", "sub_1", []byte(`{"id":"sub_1"}`))
	require.NoError(t, err)

	val, found, err := s.Get(ctx, "submissions", "sub_1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, `{"id":"sub_1"}`, string(val))

	_, found, err = s.Get(ctx, "submissions", "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestStore_DeleteRemovesKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "bucket1", "key1", []byte("val1")))
	require.NoError(t, s.Delete(ctx, "bucket1", "key1"))

	_, found, err := s.Get(ctx, "bucket1", "key1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestStore_SecondaryIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutIndexed(ctx, "dispatch_tasks", "task_1", []byte("{}"), "submission", "sub_1"))
	require.NoError(t, s.PutIndexed(ctx, "dispatch_tasks", "task_2", []byte("{}"), "submission", "sub_1"))

	keys, err := s.LookupByIndex(ctx, "dispatch_tasks", "submission", "sub_1")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	require.NoError(t, s.Delete(ctx, "dispatch_tasks", "task_1"))
	keys, err = s.LookupByIndex(ctx, "dispatch_tasks", "submission", "sub_1")
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestStore_MultipleSecondaryIndexes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tx := s.BeginTx()
	require.NoError(t, tx.Put("dispatch_tasks", "task_1", []byte("{}"), "", ""))
	tx.AddIndex("dispatch_tasks", "submission", "sub_1", "task_1")
	tx.AddIndex("dispatch_tasks", "agency", "agn_1", "task_1")
	require.NoError(t, tx.Commit(ctx))

	keys, err := s.LookupByIndex(ctx, "dispatch_tasks", "submission", "sub_1")
	require.NoError(t, err)
	assert.Contains(t, keys, "task_1")

	keys, err = s.LookupByIndex(ctx, "dispatch_tasks", "agency", "agn_1")
	require.NoError(t, err)
	assert.Contains(t, keys, "task_1")
}

func TestStore_TransactionCommit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tx := s.BeginTx()
	require.NoError(t, tx.Put("bucket1", "key1", []byte("val1"), "", ""))
	require.NoError(t, tx.Put("bucket2", "key2", []byte("val2"), "", ""))
	require.NoError(t, tx.Commit(ctx))

	val, found, _ := s.Get(ctx, "bucket1", "key1")
	assert.True(t, found)
	assert.Equal(t, "val1", string(val))

	val, found, _ = s.Get(ctx, "bucket2", "key2")
	assert.True(t, found)
	assert.Equal(t, "val2", string(val))
}

func TestStore_TransactionRollback(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tx := s.BeginTx()
	require.NoError(t, tx.Put("bucket1", "key1", []byte("val1"), "", ""))
	tx.Rollback()

	_, found, _ := s.Get(ctx, "bucket1", "key1")
	assert.False(t, found)
}

func TestStore_TransactionAtomicityCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s := NewKVStore(dir)
	require.NoError(t, s.Open(ctx))

	tx := s.BeginTx()
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		require.NoError(t, tx.Put("test_bucket", key, []byte(fmt.Sprintf("val_%d", i)), "", ""))
	}
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, s.Close())

	s2 := NewKVStore(dir)
	require.NoError(t, s2.Open(ctx))
	defer s2.Close()

	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		val, found, err := s2.Get(ctx, "test_bucket", key)
		require.NoError(t, err)
		assert.True(t, found, "key %s should survive restart", key)
		assert.Equal(t, fmt.Sprintf("val_%d", i), string(val))
	}
}

func TestStore_ReopenAndRecover(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1 := NewKVStore(dir)
	require.NoError(t, s1.Open(ctx))
	require.NoError(t, s1.Put(ctx, "submissions", "sub_1", []byte(`{"id":"sub_1"}`)))
	require.NoError(t, s1.Put(ctx, "dispatch_tasks", "dsp_1", []byte(`{"id":"dsp_1"}`)))
	require.NoError(t, s1.Close())

	s2 := NewKVStore(dir)
	require.NoError(t, s2.Open(ctx))
	defer s2.Close()

	val, found, err := s2.Get(ctx, "submissions", "sub_1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, `{"id":"sub_1"}`, string(val))

	val, found, err = s2.Get(ctx, "dispatch_tasks", "dsp_1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, `{"id":"dsp_1"}`, string(val))
}

func TestStore_PersistAcrossRestartWithIndex(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1 := NewKVStore(dir)
	require.NoError(t, s1.Open(ctx))
	require.NoError(t, s1.PutIndexed(ctx, "tasks", "t1", []byte("{}"), "submission", "s1"))
	require.NoError(t, s1.PutIndexed(ctx, "tasks", "t2", []byte("{}"), "submission", "s1"))
	require.NoError(t, s1.Close())

	s2 := NewKVStore(dir)
	require.NoError(t, s2.Open(ctx))
	defer s2.Close()

	keys, err := s2.LookupByIndex(ctx, "tasks", "submission", "s1")
	require.NoError(t, err)
	assert.Len(t, keys, 2, "secondary index should survive restart")
}

func TestStore_SnapshotPreservesData(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s := NewKVStore(dir)
	require.NoError(t, s.Open(ctx))
	for i := 0; i < 50; i++ {
		require.NoError(t, s.Put(ctx, "snap_test", fmt.Sprintf("k%d", i), []byte(fmt.Sprintf("v%d", i))))
	}
	require.NoError(t, s.Snapshot(ctx))

	for i := 0; i < 50; i++ {
		val, found, err := s.Get(ctx, "snap_test", fmt.Sprintf("k%d", i))
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, fmt.Sprintf("v%d", i), string(val))
	}

	require.NoError(t, s.Close())
	s2 := NewKVStore(dir)
	require.NoError(t, s2.Open(ctx))
	defer s2.Close()

	for i := 0; i < 50; i++ {
		val, found, err := s2.Get(ctx, "snap_test", fmt.Sprintf("k%d", i))
		require.NoError(t, err)
		assert.True(t, found, "snapshot data should survive restart")
		assert.Equal(t, fmt.Sprintf("v%d", i), string(val))
	}
}

func TestStore_ConcurrentWrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent_%d", n)
			tx := s.BeginTx()
			if err := tx.Put("concurrent_bucket", key, []byte(fmt.Sprintf("val_%d", n)), "", ""); err != nil {
				errCh <- err
				return
			}
			if err := tx.Commit(ctx); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err)
	}

	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("concurrent_%d", i)
		val, found, err := s.Get(ctx, "concurrent_bucket", key)
		require.NoError(t, err)
		assert.True(t, found, "key %s should exist after concurrent writes", key)
		assert.Equal(t, fmt.Sprintf("val_%d", i), string(val))
	}
}

func TestStore_IncompleteTransactionDiscarded(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s := NewKVStore(dir)
	require.NoError(t, s.Open(ctx))

	tx := s.BeginTx()
	require.NoError(t, tx.Put("test", "committed", []byte("yes"), "", ""))
	require.NoError(t, tx.Commit(ctx))

	require.NoError(t, s.Close())

	walPath := filepath.Join(dir, walFileName)
	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	// Write an incomplete transaction: TX_BEGIN + opPut, but no TX_COMMIT.
	// On recovery, this should be discarded since the transaction never committed.
	beginBytes, _ := encodeEntry(walEntry{Op: opTxBegin, Bucket: "tx_crash"})
	putBytes, _ := encodeEntry(walEntry{Op: opPut, Bucket: "test", Key: "uncommitted", Value: []byte("no")})
	_, err = f.Write(beginBytes)
	require.NoError(t, err)
	_, err = f.Write(putBytes)
	require.NoError(t, err)
	_ = f.Sync()
	_ = f.Close()

	s2 := NewKVStore(dir)
	require.NoError(t, s2.Open(ctx))
	defer s2.Close()

	_, found, _ := s2.Get(ctx, "test", "committed")
	assert.True(t, found, "committed data should survive")

	_, found, _ = s2.Get(ctx, "test", "uncommitted")
	assert.False(t, found, "uncommitted data should be discarded")
}

func TestStore_ScanPrefix(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "bucket", "prefix_a", []byte("1")))
	require.NoError(t, s.Put(ctx, "bucket", "prefix_b", []byte("2")))
	require.NoError(t, s.Put(ctx, "bucket", "other_c", []byte("3")))

	var matches []string
	err := s.ScanPrefix(ctx, "bucket", "prefix_", func(key string, value []byte) bool {
		matches = append(matches, key)
		return true
	})
	require.NoError(t, err)
	assert.Len(t, matches, 2)
}

func TestStore_DataVersion(t *testing.T) {
	s := newTestStore(t)
	assert.Equal(t, DataVersion, s.DataVersion())
}
