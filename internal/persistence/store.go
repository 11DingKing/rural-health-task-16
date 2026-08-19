package persistence

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DataVersion = 2

	walFileName  = "certify.wal"
	snapFileName = "certify.snapshot"
	metaFileName = "certify.meta"

	opPut      byte = 1
	opDelete   byte = 2
	opTxBegin  byte = 3
	opTxCommit byte = 4
	opSnapshot byte = 5
	opClear    byte = 6
	opIndexAdd byte = 7

	crcTableSize     = 4
	lengthPrefixSize = 4
)

// KVStore is a self-implemented append-only file-based key-value store.
// It persists to disk via a write-ahead log (WAL) and maintains an in-memory
// primary index plus secondary indexes. Crash recovery replays the WAL.
type KVStore struct {
	mu       sync.RWMutex
	dataDir  string
	walFile  *os.File
	walPath  string
	snapPath string
	metaPath string

	// primary[bucket][key] = value
	primary map[string]map[string][]byte

	// secondary[bucket][indexName][indexKey] = set of primary keys
	secondary map[string]map[string]map[string]map[string]struct{}

	// txStage accumulates ops within a transaction before commit.
	txStage map[uint64][]walEntry

	meta    storeMeta
	closed  bool
	closeMu sync.Mutex
}

type storeMeta struct {
	Version     int       `json:"version"`
	SnapshotAt  time.Time `json:"snapshot_at"`
	WalOffset   int64     `json:"wal_offset"`
	RecordCount int       `json:"record_count"`
}

type walEntry struct {
	Op        byte
	Bucket    string
	Key       string
	Value     []byte
	IndexName string
	IndexKey  string
}

// NewKVStore creates a store rooted at dataDir. Call Open to initialize.
func NewKVStore(dataDir string) *KVStore {
	return &KVStore{
		dataDir:   dataDir,
		primary:   make(map[string]map[string][]byte),
		secondary: make(map[string]map[string]map[string]map[string]struct{}),
		txStage:   make(map[uint64][]walEntry),
		meta: storeMeta{
			Version: DataVersion,
		},
	}
}

func (s *KVStore) Open(ctx context.Context) error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", s.dataDir, err)
	}
	s.walPath = filepath.Join(s.dataDir, walFileName)
	s.snapPath = filepath.Join(s.dataDir, snapFileName)
	s.metaPath = filepath.Join(s.dataDir, metaFileName)

	if err := s.loadMeta(); err != nil {
		return err
	}
	if s.meta.Version > DataVersion {
		return fmt.Errorf("data version %d is newer than supported %d", s.meta.Version, DataVersion)
	}
	if err := s.loadSnapshot(); err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	if err := s.replayWAL(ctx); err != nil {
		return fmt.Errorf("replay wal: %w", err)
	}
	f, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open wal: %w", err)
	}
	s.walFile = f
	return nil
}

func (s *KVStore) loadMeta() error {
	data, err := os.ReadFile(s.metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.meta = storeMeta{Version: DataVersion}
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.meta)
}

func (s *KVStore) saveMeta() error {
	data, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.metaPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.metaPath)
}

func (s *KVStore) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.walFile != nil {
		_ = s.walFile.Sync()
		_ = s.walFile.Close()
	}
	return s.saveMeta()
}

func (s *KVStore) Ping(ctx context.Context) error {
	if s.walFile == nil {
		return errors.New("store not open")
	}
	testKey := "__ping__"
	testDir := filepath.Dir(s.walPath)
	info, err := os.Stat(testDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("data dir is not a directory")
	}
	_ = testKey
	return nil
}

func (s *KVStore) DataVersion() int {
	return s.meta.Version
}

// DataDir returns the configured data directory.
func (s *KVStore) DataDir() string { return s.dataDir }

// Get retrieves a value by primary key.
func (s *KVStore) Get(ctx context.Context, bucket, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.primary[bucket]
	if !ok {
		return nil, false, nil
	}
	val, ok := b[key]
	return val, ok, nil
}

// Put stores a single key-value pair atomically.
func (s *KVStore) Put(ctx context.Context, bucket, key string, value []byte) error {
	tx := s.BeginTx()
	if err := tx.Put(bucket, key, value, "", ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Delete removes a key and cleans up its secondary index entries.
func (s *KVStore) Delete(ctx context.Context, bucket, key string) error {
	tx := s.BeginTx()
	tx.Delete(bucket, key)
	return tx.Commit(ctx)
}

// PutIndexed stores a value and registers a secondary index entry.
func (s *KVStore) PutIndexed(ctx context.Context, bucket, key string, value []byte, indexName, indexKey string) error {
	tx := s.BeginTx()
	if err := tx.Put(bucket, key, value, indexName, indexKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LookupByIndex returns all primary keys matching a secondary index query.
func (s *KVStore) LookupByIndex(ctx context.Context, bucket, indexName, indexKey string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.secondary[bucket]
	if !ok {
		return nil, nil
	}
	named, ok := idx[indexName]
	if !ok {
		return nil, nil
	}
	keys, ok := named[indexKey]
	if !ok {
		return nil, nil
	}
	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}
	return result, nil
}

// ScanPrefix iterates over all keys in a bucket with the given prefix.
func (s *KVStore) ScanPrefix(ctx context.Context, bucket, prefix string, fn func(key string, value []byte) bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.primary[bucket]
	if !ok {
		return nil
	}
	for k, v := range b {
		if prefix == "" || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) {
			if !fn(k, v) {
				break
			}
		}
	}
	return nil
}

// Count returns the number of keys in a bucket.
func (s *KVStore) Count(ctx context.Context, bucket string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.primary[bucket]), nil
}

// encodeEntry serializes a WAL entry to bytes with length prefix and CRC.
func encodeEntry(e walEntry) ([]byte, error) {
	buf := encodePayload(e)
	totalLen := len(buf) + crcTableSize
	out := make([]byte, lengthPrefixSize+totalLen)
	binary.BigEndian.PutUint32(out[:lengthPrefixSize], uint32(totalLen))
	copy(out[lengthPrefixSize:], buf)
	crc := crc32.ChecksumIEEE(buf)
	binary.BigEndian.PutUint32(out[lengthPrefixSize+len(buf):], crc)
	return out, nil
}

func encodePayload(e walEntry) []byte {
	var buf []byte
	buf = append(buf, e.Op)
	buf = appendString(buf, e.Bucket)
	buf = appendString(buf, e.Key)
	buf = appendBytes(buf, e.Value)
	buf = appendString(buf, e.IndexName)
	buf = appendString(buf, e.IndexKey)
	return buf
}

func appendString(buf []byte, s string) []byte {
	b := []byte(s)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(b)))
	buf = append(buf, b...)
	return buf
}

func appendBytes(buf, b []byte) []byte {
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(b)))
	buf = append(buf, b...)
	return buf
}

func readEntry(r io.Reader) (walEntry, error) {
	var lenBuf [lengthPrefixSize]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return walEntry{}, err
	}
	totalLen := binary.BigEndian.Uint32(lenBuf[:])
	if totalLen < crcTableSize || totalLen > 64*1024*1024 {
		return walEntry{}, fmt.Errorf("invalid entry length %d", totalLen)
	}
	payload := make([]byte, totalLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return walEntry{}, err
	}
	body := payload[:totalLen-crcTableSize]
	crcStored := binary.BigEndian.Uint32(payload[totalLen-crcTableSize:])
	if crc32.ChecksumIEEE(body) != crcStored {
		return walEntry{}, fmt.Errorf("crc mismatch in wal entry")
	}
	return decodePayload(body)
}

func decodePayload(buf []byte) (walEntry, error) {
	var e walEntry
	if len(buf) < 1 {
		return e, errors.New("empty payload")
	}
	e.Op = buf[0]
	buf = buf[1:]
	var err error
	if e.Bucket, buf, err = readString(buf); err != nil {
		return e, err
	}
	if e.Key, buf, err = readString(buf); err != nil {
		return e, err
	}
	if e.Value, buf, err = readBytes(buf); err != nil {
		return e, err
	}
	if e.IndexName, buf, err = readString(buf); err != nil {
		return e, err
	}
	if e.IndexKey, buf, err = readString(buf); err != nil {
		return e, err
	}
	return e, nil
}

func readString(buf []byte) (string, []byte, error) {
	b, rest, err := readBytes(buf)
	return string(b), rest, err
}

func readBytes(buf []byte) ([]byte, []byte, error) {
	if len(buf) < 4 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	n := binary.BigEndian.Uint32(buf[:4])
	if uint32(len(buf)-4) < n {
		return nil, nil, io.ErrUnexpectedEOF
	}
	return buf[4 : 4+n], buf[4+n:], nil
}
