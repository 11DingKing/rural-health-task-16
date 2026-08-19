package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// DispatchIndex resolves dispatch tasks so the SLA service can backfill
// and reconcile records. *repository.DispatchRepo satisfies this.
type DispatchIndex interface {
	Get(ctx context.Context, id string) (*domain.DispatchTask, bool, error)
	FindByStatus(ctx context.Context, status domain.DispatchStatus) ([]*domain.DispatchTask, error)
}

// AuditRecorder writes escalation events. *audit.Service satisfies this.
type AuditRecorder interface {
	Record(ctx context.Context, entityType, entityID, action, actor, detail string) error
}

// Service tracks SLA deadlines for dispatch tasks and escalates
// breaches through an explicit state machine. Records are persisted
// with secondary indexes by dispatch id and escalation level.
type Service struct {
	store      *repository.Store
	dispatches DispatchIndex
	audit      AuditRecorder
	logger     logging.Logger
	clk        clock.Clock
	policy     Policy
	mu         sync.Mutex
}

// NewService builds an SLA service.
func NewService(store *repository.Store, dispatches DispatchIndex, audit AuditRecorder, logger logging.Logger, clk clock.Clock, policy Policy) *Service {
	if clk == nil {
		clk = clock.Real()
	}
	if policy.DefaultDuration <= 0 {
		policy = DefaultPolicy()
	}
	return &Service{
		store:      store,
		dispatches: dispatches,
		audit:      audit,
		logger:     logger,
		clk:        clk,
		policy:     policy,
	}
}

// severity orders escalation levels so evaluation never downgrades a record.
func severity(l EscalationLevel) int {
	switch l {
	case LevelOnTime:
		return 0
	case LevelWarning:
		return 1
	case LevelBreached:
		return 2
	case LevelEscalated:
		return 3
	default:
		return 4 // resolved/cancelled
	}
}

// newRecord builds a fresh SLARecord from a dispatch task.
func (s *Service) newRecord(t *domain.DispatchTask) *SLARecord {
	duration := s.policy.DurationFor(t.StandardCode)
	ratio := s.policy.WarningRatioSafe()
	assigned := t.AssignedAt
	if assigned.IsZero() {
		assigned = s.clk.Now()
	}
	deadline := assigned.Add(duration)
	warning := assigned.Add(time.Duration(float64(duration) * ratio))
	now := s.clk.Now()
	return &SLARecord{
		ID:           "sla_" + uuid.NewString(),
		DispatchID:   t.ID,
		SubmissionID: t.SubmissionID,
		StandardCode: t.StandardCode,
		AgencyID:     t.AgencyID,
		AssignedAt:   assigned,
		Deadline:     deadline,
		WarningAt:    warning,
		Escalation:   LevelOnTime,
		Audit: domain.AuditMeta{
			CreatedBy: "system",
			CreatedAt: now,
			UpdatedBy: "system",
			UpdatedAt: now,
		},
	}
}

// Ensure creates an SLA record for a dispatch if none exists yet. It is
// idempotent: re-ensuring the same dispatch returns the existing record.
func (s *Service) Ensure(ctx context.Context, t *domain.DispatchTask) (*SLARecord, error) {
	if t == nil || t.ID == "" {
		return nil, fmt.Errorf("%w: dispatch task required", errorsx.ErrValidation)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok, err := s.getByDispatchLocked(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	if ok {
		return existing, nil
	}
	rec := s.newRecord(t)
	if err := s.saveRecord(ctx, rec); err != nil {
		return nil, fmt.Errorf("persist sla record: %w", err)
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, "sla", rec.ID, "ensure", "system",
			fmt.Sprintf("sla deadline %s for dispatch %s", rec.Deadline.Format("2006-01-02T15:04:05"), t.ID))
	}
	return rec, nil
}

// Evaluate progresses a record's escalation based on the current time.
// It only ever advances on_time -> warning -> breached; escalated and
// terminal records are never downgraded.
func (s *Service) Evaluate(ctx context.Context, dispatchID string) (*SLARecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok, err := s.getByDispatchLocked(ctx, dispatchID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: sla record for dispatch %s", errorsx.ErrNotFound, dispatchID)
	}
	if rec.Escalation.IsTerminal() || rec.Escalation == LevelEscalated {
		return rec, nil
	}
	target := desiredLevel(s.clk.Now(), rec.WarningAt, rec.Deadline)
	if severity(target) <= severity(rec.Escalation) {
		return rec, nil
	}
	if !rec.Escalation.CanTransitionTo(target) {
		return rec, nil
	}
	return rec, s.applyTransition(ctx, rec, target, "evaluate")
}

// Escalate manually moves a breached record to escalated. Escalating a
// non-breached record is rejected as an illegal transition.
func (s *Service) Escalate(ctx context.Context, dispatchID string) (*SLARecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok, err := s.getByDispatchLocked(ctx, dispatchID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: sla record for dispatch %s", errorsx.ErrNotFound, dispatchID)
	}
	if rec.Escalation == LevelEscalated {
		return rec, nil
	}
	if !rec.Escalation.CanTransitionTo(LevelEscalated) {
		return nil, fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, rec.Escalation, LevelEscalated)
	}
	return rec, s.applyTransition(ctx, rec, LevelEscalated, "escalate")
}

// MarkResolved transitions a record to resolved (terminal).
func (s *Service) MarkResolved(ctx context.Context, dispatchID string) error {
	return s.markTerminal(ctx, dispatchID, LevelResolved, "resolved")
}

// MarkCancelled transitions a record to cancelled (terminal).
func (s *Service) MarkCancelled(ctx context.Context, dispatchID string) error {
	return s.markTerminal(ctx, dispatchID, LevelCancelled, "cancelled")
}

func (s *Service) markTerminal(ctx context.Context, dispatchID string, target EscalationLevel, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok, err := s.getByDispatchLocked(ctx, dispatchID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: sla record for dispatch %s", errorsx.ErrNotFound, dispatchID)
	}
	if rec.Escalation == target {
		return nil
	}
	if !rec.Escalation.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, rec.Escalation, target)
	}
	return s.applyTransition(ctx, rec, target, reason)
}

// applyTransition persists the new escalation level and audits it.
func (s *Service) applyTransition(ctx context.Context, rec *SLARecord, target EscalationLevel, reason string) error {
	rec.Escalation = target
	if target.IsBreached() || target == LevelEscalated {
		now := s.clk.Now()
		rec.EscalatedAt = &now
	}
	rec.Audit.UpdatedBy = "system"
	rec.Audit.UpdatedAt = s.clk.Now()
	if err := s.saveRecord(ctx, rec); err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, "sla", rec.ID, "transition:"+reason, "system",
			fmt.Sprintf("%s -> %s", rec.DispatchID, target))
	}
	return nil
}

// ScanBreaches is the background task: it escalates every record still
// in the breached state. Returns the number escalated.
func (s *Service) ScanBreaches(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, err := s.store.LookupByIndex(ctx, BucketSLARecords, IndexEscalation, string(LevelBreached))
	if err != nil {
		return 0, err
	}
	escalated := 0
	for _, id := range keys {
		var rec SLARecord
		ok, err := s.store.GetJSON(ctx, BucketSLARecords, id, &rec)
		if err != nil || !ok {
			continue
		}
		if !rec.Escalation.CanTransitionTo(LevelEscalated) {
			continue
		}
		if err := s.applyTransition(ctx, &rec, LevelEscalated, "scan"); err != nil {
			s.logger.Error(ctx, "scan breaches escalate failed", "dispatch", rec.DispatchID, "error", err)
			continue
		}
		escalated++
	}
	if escalated > 0 && s.logger != nil {
		s.logger.Info(ctx, "sla scan escalated breaches", "count", escalated)
	}
	return escalated, nil
}

// Recompute is the restart self-heal: it backfills SLA records for any
// active dispatch that lacks one, then re-evaluates escalation for all
// records based on elapsed time. Returns the number of records affected.
func (s *Service) Recompute(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeStatuses := []domain.DispatchStatus{
		domain.DispatchAssigned,
		domain.DispatchInProgress,
		domain.DispatchRetrying,
		domain.DispatchFailover,
	}
	created := 0
	if s.dispatches != nil {
		for _, st := range activeStatuses {
			tasks, err := s.dispatches.FindByStatus(ctx, st)
			if err != nil {
				return 0, fmt.Errorf("find dispatches by status %s: %w", st, err)
			}
			for _, t := range tasks {
				_, ok, err := s.getByDispatchLocked(ctx, t.ID)
				if err != nil {
					return 0, err
				}
				if ok {
					continue
				}
				rec := s.newRecord(t)
				if err := s.saveRecord(ctx, rec); err != nil {
					return 0, err
				}
				created++
			}
		}
	}

	var recs []SLARecord
	err := s.store.ScanPrefix(ctx, BucketSLARecords, "", func(_ string, data []byte) bool {
		var r SLARecord
		if json.Unmarshal(data, &r) == nil {
			recs = append(recs, r)
		}
		return true
	})
	if err != nil {
		return created, err
	}
	now := s.clk.Now()
	updated := 0
	for i := range recs {
		r := recs[i]
		if r.Escalation.IsTerminal() || r.Escalation == LevelEscalated {
			continue
		}
		target := desiredLevel(now, r.WarningAt, r.Deadline)
		if severity(target) <= severity(r.Escalation) || !r.Escalation.CanTransitionTo(target) {
			continue
		}
		r.Escalation = target
		if target.IsBreached() {
			t := now
			r.EscalatedAt = &t
		}
		r.Audit.UpdatedAt = now
		if err := s.saveRecord(ctx, &r); err != nil {
			s.logger.Error(ctx, "recompute save failed", "dispatch", r.DispatchID, "error", err)
			continue
		}
		updated++
	}
	if created+updated > 0 && s.logger != nil {
		s.logger.Info(ctx, "sla recompute", "created", created, "updated", updated)
	}
	return created + updated, nil
}

// ListBreaches returns paginated SLA records. When escalationFilter is
// empty it returns warning/breached/escalated records; otherwise it
// filters to the requested level.
func (s *Service) ListBreaches(ctx context.Context, params domain.PaginationParams, escalationFilter string) ([]SLARecord, int, error) {
	params = params.Normalize()
	var all []SLARecord
	err := s.store.ScanPrefix(ctx, BucketSLARecords, "", func(_ string, data []byte) bool {
		var r SLARecord
		if json.Unmarshal(data, &r) != nil {
			return true
		}
		if escalationFilter != "" {
			if string(r.Escalation) != escalationFilter {
				return true
			}
		} else if !r.Escalation.IsBreached() && r.Escalation != LevelWarning {
			return true
		}
		all = append(all, r)
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Deadline.Before(all[j].Deadline) })
	total := len(all)
	start := params.Offset()
	if start >= total {
		return []SLARecord{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// CountByEscalation returns the number of records at a given level.
func (s *Service) CountByEscalation(ctx context.Context, level EscalationLevel) (int, error) {
	keys, err := s.store.LookupByIndex(ctx, BucketSLARecords, IndexEscalation, string(level))
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// GetByDispatch returns the SLA record for a dispatch, if any.
func (s *Service) GetByDispatch(ctx context.Context, dispatchID string) (*SLARecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getByDispatchLocked(ctx, dispatchID)
}

func (s *Service) getByDispatchLocked(ctx context.Context, dispatchID string) (*SLARecord, bool, error) {
	keys, err := s.store.LookupByIndex(ctx, BucketSLARecords, IndexDispatch, dispatchID)
	if err != nil {
		return nil, false, err
	}
	if len(keys) == 0 {
		return nil, false, nil
	}
	var rec SLARecord
	ok, err := s.store.GetJSON(ctx, BucketSLARecords, keys[0], &rec)
	if err != nil || !ok {
		return nil, false, err
	}
	return &rec, true, nil
}

func (s *Service) saveRecord(ctx context.Context, rec *SLARecord) error {
	indexes := []repository.IndexEntry{
		{Name: IndexDispatch, Key: rec.DispatchID},
		{Name: IndexEscalation, Key: string(rec.Escalation)},
	}
	return s.store.PutJSON(ctx, BucketSLARecords, rec.ID, rec, indexes)
}
