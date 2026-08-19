package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ruralhealth/internal/healthdb"
)

func TestTokenLifecycle(t *testing.T) {
	db, err := healthdb.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(db, time.Hour)
	if _, err := a.CreateUser(context.Background(), "doctor", "password1", RoleVillageDoctor, "v1"); err != nil {
		t.Fatal(err)
	}
	token, user, _, err := a.Login(context.Background(), "doctor", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != RoleVillageDoctor {
		t.Fatalf("role=%s", user.Role)
	}
	if _, err := a.Authenticate(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if err := a.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, healthdb.ErrRevoked) {
		t.Fatalf("expected revoked, got %v", err)
	}
}
