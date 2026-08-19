package healthdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("health record not found")
var ErrConflict = errors.New("health record version conflict")
var ErrRevoked = errors.New("session revoked")

type DB struct{ sql *sql.DB }

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open health database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	h := &DB{sql: db}
	if err := h.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return h, nil
}

func (d *DB) Close() error                   { return d.sql.Close() }
func (d *DB) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }

func (d *DB) Migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_versions(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, role TEXT NOT NULL, village_id TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), expires_at TEXT NOT NULL, revoked_at TEXT, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at, revoked_at);
CREATE TABLE IF NOT EXISTS villagers(id TEXT PRIMARY KEY, village_id TEXT NOT NULL, name TEXT NOT NULL, phone TEXT NOT NULL, birth_date TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS chronic_conditions(id TEXT PRIMARY KEY, villager_id TEXT NOT NULL REFERENCES villagers(id), kind TEXT NOT NULL, target_systolic INTEGER NOT NULL, target_diastolic INTEGER NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(villager_id, kind));
CREATE TABLE IF NOT EXISTS care_contracts(id TEXT PRIMARY KEY, villager_id TEXT NOT NULL REFERENCES villagers(id), doctor_id TEXT NOT NULL REFERENCES users(id), status TEXT NOT NULL, signed_at TEXT NOT NULL, UNIQUE(villager_id, status));
CREATE TABLE IF NOT EXISTS visits(id TEXT PRIMARY KEY, villager_id TEXT NOT NULL REFERENCES villagers(id), doctor_id TEXT NOT NULL REFERENCES users(id), kind TEXT NOT NULL, scheduled_at TEXT NOT NULL, status TEXT NOT NULL, systolic INTEGER, diastolic INTEGER, glucose REAL, note TEXT NOT NULL, idempotency_key TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS referrals(id TEXT PRIMARY KEY, villager_id TEXT NOT NULL REFERENCES villagers(id), from_doctor TEXT NOT NULL REFERENCES users(id), to_doctor TEXT NOT NULL REFERENCES users(id), reason TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(villager_id, status));
CREATE TABLE IF NOT EXISTS medicines(id TEXT PRIMARY KEY, village_id TEXT NOT NULL, name TEXT NOT NULL, quantity INTEGER NOT NULL, reorder_level INTEGER NOT NULL, version INTEGER NOT NULL DEFAULT 1, updated_at TEXT NOT NULL, UNIQUE(village_id, name));
CREATE TABLE IF NOT EXISTS audit_events(id INTEGER PRIMARY KEY AUTOINCREMENT, actor_id TEXT NOT NULL, action TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, request_id TEXT NOT NULL, details TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS villagers_village_idx ON villagers(village_id, updated_at);
CREATE INDEX IF NOT EXISTS visits_villager_idx ON visits(villager_id, scheduled_at);
CREATE INDEX IF NOT EXISTS audit_entity_idx ON audit_events(entity_type, entity_id, created_at);
`
	if _, err := d.sql.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate health database: %w", err)
	}
	_, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO schema_versions(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (d *DB) SQL() *sql.DB { return d.sql }

type Tx struct{ *sql.Tx }

func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}
func (t *Tx) RollbackIfOpen() { _ = t.Rollback() }

func (d *DB) RecordAudit(ctx context.Context, actor, action, typ, id, requestID, details string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,request_id,details,created_at) VALUES(?,?,?,?,?,?,?)`, actor, action, typ, id, requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
