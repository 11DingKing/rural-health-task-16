package persistence

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
)

// replayWAL replays the write-ahead log to rebuild in-memory state.
// Only complete TX_BEGIN...TX_COMMIT segments are applied; partial
// segments (crash during write) are discarded.
func (s *KVStore) replayWAL(ctx context.Context) error {
	f, err := os.Open(s.walPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	pending := make([]walEntry, 0, 64)
	inTx := false
	recovered := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := readEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				break
			}
			return fmt.Errorf("read wal entry: %w", err)
		}

		switch entry.Op {
		case opTxBegin:
			inTx = true
			pending = pending[:0]
		case opTxCommit:
			if inTx {
				for _, e := range pending {
					s.applyEntry(e)
				}
				recovered += len(pending)
				pending = pending[:0]
				inTx = false
			}
		case opPut, opDelete, opIndexAdd:
			if inTx {
				pending = append(pending, entry)
			} else {
				s.applyEntry(entry)
				recovered++
			}
		case opSnapshot:
			s.meta.RecordCount = 0
		case opClear:
			s.primary = make(map[string]map[string][]byte)
			s.secondary = make(map[string]map[string]map[string]map[string]struct{})
		}
	}
	if inTx {
		// Incomplete transaction at end of WAL — discard.
		pending = pending[:0]
	}
	return nil
}

// loadSnapshot loads a snapshot file if it exists and populates in-memory state.
func (s *KVStore) loadSnapshot() error {
	f, err := os.Open(s.snapPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		entry, err := readEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read snapshot: %w", err)
		}
		if entry.Op == opPut {
			s.applyEntry(entry)
		}
	}
	return nil
}

// Snapshot writes the current in-memory state to a snapshot file and
// truncates the WAL. This prevents unbounded WAL growth.
func (s *KVStore) Snapshot(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	tmp := s.snapPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	writer := newBufferedWriter(f)

	for bucket, keys := range s.primary {
		for key, val := range keys {
			entry := walEntry{Op: opPut, Bucket: bucket, Key: key, Value: val}
			data, err := encodeEntry(entry)
			if err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			if _, err := writer.Write(data); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
		}
	}
	if err := writer.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.snapPath); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}

	// Truncate WAL since snapshot covers everything.
	if s.walFile != nil {
		if err := s.walFile.Close(); err != nil {
			return err
		}
		if err := os.Truncate(s.walPath, 0); err != nil {
			return err
		}
		f2, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		s.walFile = f2
	}

	s.meta.SnapshotAt = s.now()
	s.meta.WalOffset = 0
	return s.saveMeta()
}

// now returns current time — injected in tests via clock.
func (s *KVStore) now() (t timeNow) {
	return timeNowFunc()
}
