package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// SubmissionIndex resolves submission status so the quota service can
// self-heal reservations whose submission became terminal. The concrete
// *repository.SubmissionRepo satisfies this structurally.
type SubmissionIndex interface {
	Get(ctx context.Context, id string) (*domain.Submission, bool, error)
}

// AuditRecorder writes quota events to the audit log. *audit.Service
// satisfies this structurally; tests may pass a no-op recorder.
type AuditRecorder interface {
	Record(ctx context.Context, entityType, entityID, action, actor, detail string) error
}

// Service enforces per-enterprise active-submission quotas backed by the
// file KV store. Reservations are persisted with a secondary index by
// enterprise so they survive restarts; Recompute reconciles them against
// actual submission status on startup.
type Service struct {
	store       *repository.Store
	submissions SubmissionIndex
	audit       AuditRecorder
	logger      logging.Logger
	clk         clock.Clock
	policy      Policy
	mu          sync.Mutex
}

// NewService builds a quota service.
func NewService(store *repository.Store, submissions SubmissionIndex, audit AuditRecorder, logger logging.Logger, clk clock.Clock, policy Policy) *Service {
	if clk == nil {
		clk = clock.Real()
	}
	if policy.MaxActivePerEnterprise <= 0 {
		policy = DefaultPolicy()
	}
	return &Service{
		store:       store,
		submissions: submissions,
		audit:       audit,
		logger:      logger,
		clk:         clk,
		policy:      policy,
	}
}

// Policy returns the active quota policy.
func (s *Service) Policy() Policy { return s.policy }

// Acquire reserves a quota slot for an enterprise. It returns
// ErrQuotaExceeded (wrapped) when the enterprise already holds the
// maximum number of active submissions. Acquire is serialized so
// concurrent callers cannot overrun the limit.
func (s *Service) Acquire(ctx context.Context, enterpriseID string) (*Reservation, error) {
	if enterpriseID == "" {
		return nil, fmt.Errorf("%w: enterprise_id required", errorsx.ErrValidation)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	count, err := s.countActiveLocked(ctx, enterpriseID)
	if err != nil {
		return nil, fmt.Errorf("count active reservations: %w", err)
	}
	if count >= s.policy.MaxActivePerEnterprise {
		return nil, fmt.Errorf("%w: enterprise %s holds %d active submissions (max %d)",
			errorsx.ErrQuotaExceeded, enterpriseID, count, s.policy.MaxActivePerEnterprise)
	}

	now := s.clk.Now()
	res := &Reservation{
		ID:           "qta_" + uuid.NewString(),
		EnterpriseID: enterpriseID,
		AcquiredAt:   now,
		Audit: domain.AuditMeta{
			CreatedBy: enterpriseID,
			CreatedAt: now,
			UpdatedBy: enterpriseID,
			UpdatedAt: now,
		},
	}
	if err := s.saveReservation(ctx, res); err != nil {
		return nil, fmt.Errorf("persist reservation: %w", err)
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, "quota", res.ID, "acquire", enterpriseID,
			fmt.Sprintf("acquired slot %d/%d", count+1, s.policy.MaxActivePerEnterprise))
	}
	return res, nil
}

// Bind associates a created submission with a previously acquired
// reservation. Called after the submission is durably stored.
func (s *Service) Bind(ctx context.Context, reservationID, submissionID, modelNo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok, err := s.getReservation(ctx, reservationID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: reservation %s", errorsx.ErrNotFound, reservationID)
	}
	res.SubmissionID = submissionID
	if modelNo != "" {
		res.ModelNo = modelNo
	}
	res.Audit.UpdatedBy = "system"
	res.Audit.UpdatedAt = s.clk.Now()
	return s.saveReservation(ctx, res)
}

// Release marks a reservation as no longer consuming a slot. Releasing
// an already-released reservation is a no-op (idempotent).
func (s *Service) Release(ctx context.Context, reservationID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok, err := s.getReservation(ctx, reservationID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: reservation %s", errorsx.ErrNotFound, reservationID)
	}
	return s.releaseLocked(ctx, res, reason)
}

// ReleaseBySubmission releases the reservation bound to a submission.
// Used when a submission reaches a terminal state.
func (s *Service) ReleaseBySubmission(ctx context.Context, submissionID, reason string) error {
	if submissionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, err := s.store.LookupByIndex(ctx, BucketQuotaReservations, IndexSubmission, submissionID)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	for _, id := range keys {
		res, ok, err := s.getReservation(ctx, id)
		if err != nil || !ok {
			continue
		}
		if err := s.releaseLocked(ctx, res, reason); err != nil {
			return err
		}
	}
	return nil
}

// CountActive returns the number of active reservations for an enterprise.
func (s *Service) CountActive(ctx context.Context, enterpriseID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.countActiveLocked(ctx, enterpriseID)
}

func (s *Service) countActiveLocked(ctx context.Context, enterpriseID string) (int, error) {
	keys, err := s.store.LookupByIndex(ctx, BucketQuotaReservations, IndexEnterprise, enterpriseID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range keys {
		res, ok, err := s.getReservation(ctx, id)
		if err != nil {
			return 0, err
		}
		if ok && res.IsActive() {
			count++
		}
	}
	return count, nil
}

// GetUsage returns the current quota usage for an enterprise.
func (s *Service) GetUsage(ctx context.Context, enterpriseID string) (Usage, error) {
	count, err := s.CountActive(ctx, enterpriseID)
	if err != nil {
		return Usage{}, err
	}
	return Usage{
		EnterpriseID: enterpriseID,
		ActiveCount:  count,
		MaxActive:    s.policy.MaxActivePerEnterprise,
	}, nil
}

// ListUsage returns paginated per-enterprise usage. Enterprises are
// ordered by id so paging is deterministic across calls.
func (s *Service) ListUsage(ctx context.Context, params domain.PaginationParams) ([]Usage, int, error) {
	params = params.Normalize()
	counts := map[string]int{}
	err := s.store.ScanPrefix(ctx, BucketQuotaReservations, "", func(_ string, data []byte) bool {
		var res Reservation
		if json.Unmarshal(data, &res) == nil && res.IsActive() {
			counts[res.EnterpriseID]++
		}
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	enterprises := make([]string, 0, len(counts))
	for ent := range counts {
		enterprises = append(enterprises, ent)
	}
	sort.Strings(enterprises)
	total := len(enterprises)
	start := params.Offset()
	if start >= total {
		return []Usage{}, total, nil
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}
	page := make([]Usage, 0, end-start)
	for _, ent := range enterprises[start:end] {
		page = append(page, Usage{
			EnterpriseID: ent,
			ActiveCount:  counts[ent],
			MaxActive:    s.policy.MaxActivePerEnterprise,
		})
	}
	return page, total, nil
}

// Recompute is the restart self-heal: it walks every active reservation
// and releases any whose submission is missing or terminal. This
// restores the invariant that active reservations equal active
// submissions even if the process crashed between a status change and a
// reservation release.
func (s *Service) Recompute(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var active []Reservation
	err := s.store.ScanPrefix(ctx, BucketQuotaReservations, "", func(_ string, data []byte) bool {
		var res Reservation
		if json.Unmarshal(data, &res) == nil && res.IsActive() {
			active = append(active, res)
		}
		return true
	})
	if err != nil {
		return 0, err
	}

	reconciled := 0
	for i := range active {
		res := active[i]
		reason := s.reconcileReason(ctx, &res)
		if reason == "" {
			continue
		}
		if err := s.releaseLocked(ctx, &res, reason); err != nil {
			s.logger.Error(ctx, "recompute release failed", "reservation", res.ID, "error", err)
			continue
		}
		reconciled++
	}
	if reconciled > 0 && s.logger != nil {
		s.logger.Info(ctx, "quota recompute reconciled reservations", "count", reconciled)
	}
	return reconciled, nil
}

func (s *Service) reconcileReason(ctx context.Context, res *Reservation) string {
	if res.SubmissionID == "" {
		return "orphan_unbound"
	}
	if s.submissions == nil {
		return ""
	}
	sub, found, err := s.submissions.Get(ctx, res.SubmissionID)
	if err != nil {
		return "lookup_error"
	}
	if !found {
		return "submission_missing"
	}
	if sub.Status.IsTerminal() {
		return "submission_terminal"
	}
	return ""
}

// SweepExpired is the background housekeeping task: it deletes released
// reservations older than the retention period so the bucket stays
// bounded. It is Ticker-driven and safe to run concurrently with
// Acquire/Release because it only touches released records.
func (s *Service) SweepExpired(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.clk.Now().Add(-s.policy.ReservationRetention)

	var stale []string
	err := s.store.ScanPrefix(ctx, BucketQuotaReservations, "", func(_ string, data []byte) bool {
		var res Reservation
		if json.Unmarshal(data, &res) != nil {
			return true
		}
		if res.ReleasedAt != nil && res.ReleasedAt.Before(cutoff) {
			stale = append(stale, res.ID)
		}
		return true
	})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, id := range stale {
		if err := s.store.Delete(ctx, BucketQuotaReservations, id); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func (s *Service) releaseLocked(ctx context.Context, res *Reservation, reason string) error {
	if !res.IsActive() {
		return nil
	}
	now := s.clk.Now()
	res.ReleasedAt = &now
	res.ReleasedReason = reason
	res.Audit.UpdatedBy = "system"
	res.Audit.UpdatedAt = now
	if err := s.saveReservation(ctx, res); err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, "quota", res.ID, "release", res.EnterpriseID, reason)
	}
	return nil
}

func (s *Service) getReservation(ctx context.Context, id string) (*Reservation, bool, error) {
	var res Reservation
	ok, err := s.store.GetJSON(ctx, BucketQuotaReservations, id, &res)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return &res, true, nil
}

func (s *Service) saveReservation(ctx context.Context, res *Reservation) error {
	indexes := []repository.IndexEntry{{Name: IndexEnterprise, Key: res.EnterpriseID}}
	if res.SubmissionID != "" {
		indexes = append(indexes, repository.IndexEntry{Name: IndexSubmission, Key: res.SubmissionID})
	}
	return s.store.PutJSON(ctx, BucketQuotaReservations, res.ID, res, indexes)
}
