package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"ruralhealth/internal/auth"
	"ruralhealth/internal/healthdb"
)

type Service struct {
	db   *healthdb.DB
	auth *auth.Service
	now  func() time.Time
}

func New(db *healthdb.DB, authn *auth.Service) *Service {
	return &Service{db: db, auth: authn, now: time.Now}
}

type Villager struct {
	ID, VillageID, Name, Phone, BirthDate string
	Version                               int
}
type Visit struct {
	ID, VillagerID, DoctorID, Kind, ScheduledAt, Status, Note string
	Systolic, Diastolic                                       *int
	Glucose                                                   *float64
}
type Referral struct {
	ID, VillagerID, FromDoctor, ToDoctor, Reason, Status string
	Version                                              int
}

func (s *Service) CreateVillager(ctx context.Context, actor auth.User, name, phone, birthDate string) (Villager, error) {
	if actor.Role != auth.RoleVillageDoctor && actor.Role != auth.RoleTownDoctor && actor.Role != auth.RoleAdmin {
		return Villager{}, errors.New("role cannot create villager")
	}
	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.SQL().ExecContext(ctx, `INSERT INTO villagers(id,village_id,name,phone,birth_date,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, actor.VillageID, name, phone, birthDate, now, now)
	if err != nil {
		return Villager{}, fmt.Errorf("create villager: %w", err)
	}
	return Villager{ID: id, VillageID: actor.VillageID, Name: name, Phone: phone, BirthDate: birthDate, Version: 1}, nil
}

func (s *Service) RecordVisit(ctx context.Context, actor auth.User, villagerID, kind, scheduled, note, idem string, systolic, diastolic *int, glucose *float64) (Visit, error) {
	if idem == "" {
		return Visit{}, errors.New("idempotency key required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Visit{}, err
	}
	defer tx.RollbackIfOpen()
	var existing Visit
	var sys, dia sql.NullInt64
	var glu sql.NullFloat64
	err = tx.QueryRow(`SELECT id,villager_id,doctor_id,kind,scheduled_at,status,systolic,diastolic,glucose,note FROM visits WHERE idempotency_key=?`, idem).Scan(&existing.ID, &existing.VillagerID, &existing.DoctorID, &existing.Kind, &existing.ScheduledAt, &existing.Status, &sys, &dia, &glu, &existing.Note)
	if err == nil {
		if sys.Valid {
			v := int(sys.Int64)
			existing.Systolic = &v
		}
		if dia.Valid {
			v := int(dia.Int64)
			existing.Diastolic = &v
		}
		if glu.Valid {
			v := glu.Float64
			existing.Glucose = &v
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Visit{}, fmt.Errorf("check visit idempotency: %w", err)
	}
	var village string
	if err := tx.QueryRow(`SELECT village_id FROM villagers WHERE id=?`, villagerID).Scan(&village); err != nil {
		return Visit{}, fmt.Errorf("load villager: %w", err)
	}
	if village != actor.VillageID && actor.Role != auth.RoleCountyExpert && actor.Role != auth.RoleAdmin {
		return Visit{}, errors.New("villager belongs to another village")
	}
	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`INSERT INTO visits(id,villager_id,doctor_id,kind,scheduled_at,status,systolic,diastolic,glucose,note,idempotency_key,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, villagerID, actor.ID, kind, scheduled, "completed", systolic, diastolic, glucose, note, idem, now)
	if err != nil {
		return Visit{}, fmt.Errorf("record visit: %w", err)
	}
	details, _ := json.Marshal(map[string]any{"kind": kind, "scheduled_at": scheduled})
	if _, err = tx.Exec(`INSERT INTO audit_events(actor_id,action,entity_type,entity_id,request_id,details,created_at) VALUES(?,?,?,?,?,?,?)`, actor.ID, "record_visit", "visit", id, idem, string(details), now); err != nil {
		return Visit{}, fmt.Errorf("audit visit: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Visit{}, fmt.Errorf("commit visit: %w", err)
	}
	return Visit{ID: id, VillagerID: villagerID, DoctorID: actor.ID, Kind: kind, ScheduledAt: scheduled, Status: "completed", Note: note, Systolic: systolic, Diastolic: diastolic, Glucose: glucose}, nil
}

func (s *Service) CreateReferral(ctx context.Context, actor auth.User, villagerID, toDoctor, reason string) (Referral, error) {
	if actor.Role != auth.RoleVillageDoctor && actor.Role != auth.RoleTownDoctor && actor.Role != auth.RoleCountyExpert {
		return Referral{}, errors.New("role cannot refer")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Referral{}, err
	}
	defer tx.RollbackIfOpen()
	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.Exec(`INSERT INTO referrals(id,villager_id,from_doctor,to_doctor,reason,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, villagerID, actor.ID, toDoctor, reason, "pending", now, now); err != nil {
		return Referral{}, fmt.Errorf("create referral: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO audit_events(actor_id,action,entity_type,entity_id,request_id,details,created_at) VALUES(?,?,?,?,?,?,?)`, actor.ID, "create_referral", "referral", id, id, reason, now); err != nil {
		return Referral{}, fmt.Errorf("audit referral: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Referral{}, err
	}
	return Referral{ID: id, VillagerID: villagerID, FromDoctor: actor.ID, ToDoctor: toDoctor, Reason: reason, Status: "pending", Version: 1}, nil
}

func (s *Service) AcceptReferral(ctx context.Context, actor auth.User, id string, version int) error {
	if actor.Role != auth.RoleCountyExpert && actor.Role != auth.RoleTownDoctor {
		return errors.New("role cannot accept referral")
	}
	result, err := s.db.SQL().ExecContext(ctx, `UPDATE referrals SET status='accepted',version=version+1,updated_at=? WHERE id=? AND version=? AND to_doctor=? AND status='pending'`, s.now().UTC().Format(time.RFC3339Nano), id, version, actor.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return healthdb.ErrConflict
	}
	return nil
}
