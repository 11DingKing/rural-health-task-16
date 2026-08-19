package health

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ruralhealth/internal/auth"
	"ruralhealth/internal/healthdb"
)

func TestVisitIdempotencyAndReferralVersion(t *testing.T) {
	db, err := healthdb.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := auth.New(db, time.Hour)
	doctor, err := a.CreateUser(context.Background(), "vdoctor", "password1", auth.RoleVillageDoctor, "v1")
	if err != nil {
		t.Fatal(err)
	}
	expert, err := a.CreateUser(context.Background(), "expert", "password2", auth.RoleCountyExpert, "v1")
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, a)
	p, err := s.CreateVillager(context.Background(), doctor, "张大爷", "13800000000", "1950-01-01")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.RecordVisit(context.Background(), doctor, p.ID, "home_followup", "2026-08-20T09:00:00Z", "血压稳定", "idem-1", intPtr(135), intPtr(82), nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := s.RecordVisit(context.Background(), doctor, p.ID, "home_followup", "2026-08-20T09:00:00Z", "ignored", "idem-1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != v2.ID {
		t.Fatal("idempotency returned a different visit")
	}
	r, err := s.CreateReferral(context.Background(), doctor, p.ID, expert.ID, "需要远程会诊")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptReferral(context.Background(), expert, r.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptReferral(context.Background(), expert, r.ID, 1); err == nil {
		t.Fatal("stale referral version accepted")
	}
}
func intPtr(v int) *int { return &v }
