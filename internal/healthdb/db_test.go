package healthdb

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrationIsRestartSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.db")
	one, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := one.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := one.Close(); err != nil {
		t.Fatal(err)
	}
	two, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()
	var n int
	if err := two.SQL().QueryRow(`SELECT COUNT(*) FROM schema_versions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema versions=%d", n)
	}
}

func TestTransactionRollsBackCrossEntityWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO users(id,username,password_hash,role,village_id,created_at) VALUES('u','doctor','x','village_doctor','v1','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO villagers(id,village_id,name,phone,birth_date,created_at,updated_at) VALUES('p','v1','Li','1','1970-01-01','now','now')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rollback left users=%d", n)
	}
}
