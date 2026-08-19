package persistence

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
)

var txCounter uint64

// Transaction accumulates operations committed atomically via WAL.
type Transaction struct {
	store   *KVStore
	entries []walEntry
	txID    uint64
	aborted bool
}

func (s *KVStore) BeginTx() *Transaction {
	id := atomic.AddUint64(&txCounter, 1)
	return &Transaction{store: s, txID: id}
}

// Put queues a key-value write with an optional secondary index entry.
func (tx *Transaction) Put(bucket, key string, value []byte, indexName, indexKey string) error {
	if tx.aborted {
		return errors.New("transaction already aborted")
	}
	tx.entries = append(tx.entries, walEntry{
		Op:        opPut,
		Bucket:    bucket,
		Key:       key,
		Value:     append([]byte(nil), value...),
		IndexName: indexName,
		IndexKey:  indexKey,
	})
	return nil
}

// AddIndex queues a secondary index entry without storing a value.
// This enables multiple indexes per record.
func (tx *Transaction) AddIndex(bucket, indexName, indexKey, primaryKey string) {
	if tx.aborted {
		return
	}
	tx.entries = append(tx.entries, walEntry{
		Op:        opIndexAdd,
		Bucket:    bucket,
		Key:       primaryKey,
		IndexName: indexName,
		IndexKey:  indexKey,
	})
}

// Delete queues a key deletion. All associated index entries are removed.
func (tx *Transaction) Delete(bucket, key string) {
	if tx.aborted {
		return
	}
	tx.entries = append(tx.entries, walEntry{
		Op:     opDelete,
		Bucket: bucket,
		Key:    key,
	})
}

func (tx *Transaction) Len() int { return len(tx.entries) }

func (tx *Transaction) Rollback() {
	tx.aborted = true
	tx.entries = nil
}

func (tx *Transaction) Commit(ctx context.Context) error {
	if tx.aborted {
		return errors.New("cannot commit aborted transaction")
	}
	if len(tx.entries) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	if tx.store.closed {
		return ErrStoreClosed
	}

	segment := tx.buildSegment()
	if _, err := tx.store.walFile.Write(segment); err != nil {
		return fmt.Errorf("write wal segment: %w", err)
	}
	if err := tx.store.walFile.Sync(); err != nil {
		return fmt.Errorf("sync wal: %w", err)
	}

	for _, e := range tx.entries {
		tx.store.applyEntry(e)
	}
	tx.store.meta.RecordCount += countNetEntries(tx.entries)
	tx.entries = nil
	return nil
}

func countNetEntries(entries []walEntry) int {
	net := 0
	for _, e := range entries {
		switch e.Op {
		case opPut:
			net++
		case opDelete:
			net--
		}
	}
	return net
}

func (tx *Transaction) buildSegment() []byte {
	var segment []byte
	begin := walEntry{Op: opTxBegin, Bucket: fmt.Sprintf("tx_%d", tx.txID)}
	beginBytes, _ := encodeEntry(begin)
	segment = append(segment, beginBytes...)

	for _, e := range tx.entries {
		b, _ := encodeEntry(e)
		segment = append(segment, b...)
	}

	commit := walEntry{Op: opTxCommit, Bucket: fmt.Sprintf("tx_%d", tx.txID)}
	commitBytes, _ := encodeEntry(commit)
	segment = append(segment, commitBytes...)
	return segment
}

// applyEntry mutates in-memory state for a single WAL entry.
func (s *KVStore) applyEntry(e walEntry) {
	switch e.Op {
	case opPut:
		if s.primary[e.Bucket] == nil {
			s.primary[e.Bucket] = make(map[string][]byte)
		}
		// Clean up old index entries for this key before overwriting.
		s.removeAllIndexesForKey(e.Bucket, e.Key)
		s.primary[e.Bucket][e.Key] = e.Value
		if e.IndexName != "" {
			s.addIndexEntry(e.Bucket, e.IndexName, e.IndexKey, e.Key)
		}
	case opIndexAdd:
		s.addIndexEntry(e.Bucket, e.IndexName, e.IndexKey, e.Key)
	case opDelete:
		if s.primary[e.Bucket] != nil {
			s.removeAllIndexesForKey(e.Bucket, e.Key)
			delete(s.primary[e.Bucket], e.Key)
		}
	}
}

// removeAllIndexesForKey scans all secondary indexes for a bucket and
// removes any entries pointing to the given primary key.
func (s *KVStore) removeAllIndexesForKey(bucket, primaryKey string) {
	b, ok := s.secondary[bucket]
	if !ok {
		return
	}
	for _, named := range b {
		for indexKey, keys := range named {
			if _, exists := keys[primaryKey]; exists {
				delete(keys, primaryKey)
				if len(keys) == 0 {
					delete(named, indexKey)
				}
			}
		}
	}
}

var ErrStoreClosed = errors.New("store is closed")

func encodeUint64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (s *KVStore) addIndexEntry(bucket, indexName, indexKey, primaryKey string) {
	if s.secondary[bucket] == nil {
		s.secondary[bucket] = make(map[string]map[string]map[string]struct{})
	}
	if s.secondary[bucket][indexName] == nil {
		s.secondary[bucket][indexName] = make(map[string]map[string]struct{})
	}
	if s.secondary[bucket][indexName][indexKey] == nil {
		s.secondary[bucket][indexName][indexKey] = make(map[string]struct{})
	}
	s.secondary[bucket][indexName][indexKey][primaryKey] = struct{}{}
}
