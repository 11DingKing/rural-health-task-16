package consistency

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// Service maintains the consistency between detail test results and
// model summary conclusions. When details change, the summary is
// immediately recomputed. If details diverge from the summary, the
// model is frozen for review.
type Service struct {
	summaryRepo    *repository.SummaryRepo
	resultRepo     *repository.ResultRepo
	dispatchRepo   *repository.DispatchRepo
	submissionRepo *repository.SubmissionRepo
	logger         logging.Logger
	clk            clock.Clock
	sm             SummaryStateMachine
}

type SummaryStateMachine struct{}

func (ssm SummaryStateMachine) Transition(current, target domain.SummaryStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func NewService(
	summaryRepo *repository.SummaryRepo,
	resultRepo *repository.ResultRepo,
	dispatchRepo *repository.DispatchRepo,
	submissionRepo *repository.SubmissionRepo,
	logger logging.Logger,
	clk clock.Clock,
) *Service {
	return &Service{
		summaryRepo:    summaryRepo,
		resultRepo:     resultRepo,
		dispatchRepo:   dispatchRepo,
		submissionRepo: submissionRepo,
		logger:         logger,
		clk:            clk,
		sm:             SummaryStateMachine{},
	}
}

// RecomputeSummary recalculates the aggregate conclusion for a submission
// based on all detail results. Called whenever a detail result changes.
func (s *Service) RecomputeSummary(ctx context.Context, submissionID string) (*domain.Summary, error) {
	sub, found, err := s.submissionRepo.Get(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}

	results, err := s.resultRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}

	tasks, err := s.dispatchRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	totalStandards := len(tasks)
	completedCount := 0
	passedCount := 0
	failedCount := 0
	for _, r := range results {
		completedCount++
		if r.Passed {
			passedCount++
		} else {
			failedCount++
		}
	}

	now := s.clk.Now()
	existing, err := s.summaryRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}

	overallPassed := completedCount > 0 && passedCount == completedCount && failedCount == 0

	if existing == nil {
		summary := &domain.Summary{
			ID:            "sum_" + uuid.NewString(),
			ModelNo:       sub.ModelNo,
			BatchNo:       sub.BatchNo,
			SubmissionID:  submissionID,
			OverallPassed: overallPassed,
			StandardCount: totalStandards,
			PassedCount:   passedCount,
			FailedCount:   failedCount,
			Status:        domain.SummaryComputed,
			ComputedAt:    now,
			Version:       1,
			Audit: domain.AuditMeta{
				CreatedBy: "system",
				CreatedAt: now,
				UpdatedBy: "system",
				UpdatedAt: now,
			},
		}
		if err := s.summaryRepo.Save(ctx, summary); err != nil {
			return nil, err
		}
		return summary, nil
	}

	existing.Version++
	existing.OverallPassed = overallPassed
	existing.StandardCount = totalStandards
	existing.PassedCount = passedCount
	existing.FailedCount = failedCount
	existing.ComputedAt = now
	existing.Audit.UpdatedAt = now
	if existing.Status == domain.SummaryStale {
		if err := s.sm.Transition(existing.Status, domain.SummaryComputed); err != nil {
			return nil, err
		}
		existing.Status = domain.SummaryComputed
	}
	if err := s.summaryRepo.Save(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// CheckConsistency verifies that detail conclusions match the summary.
// If they diverge, the model is frozen for review.
func (s *Service) CheckConsistency(ctx context.Context, submissionID string) (*domain.ConsistencyCheck, error) {
	summary, err := s.summaryRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return &domain.ConsistencyCheck{
			ModelID:    submissionID,
			Consistent: true,
			CheckedAt:  s.clk.Now(),
		}, nil
	}

	results, err := s.resultRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}

	actualPassed := 0
	actualFailed := 0
	for _, r := range results {
		if r.Passed {
			actualPassed++
		} else {
			actualFailed++
		}
	}

	consistent := actualPassed == summary.PassedCount && actualFailed == summary.FailedCount
	check := &domain.ConsistencyCheck{
		ModelID:      summary.ModelNo,
		DetailCount:  len(results),
		SummaryCount: summary.PassedCount + summary.FailedCount,
		Consistent:   consistent,
		CheckedAt:    s.clk.Now(),
	}

	if !consistent {
		check.DivergenceNote = fmt.Sprintf("detail=%d/%d but summary=%d/%d",
			actualPassed, actualFailed, summary.PassedCount, summary.FailedCount)
		if err := s.FreezeForReview(ctx, summary.ModelNo, "consistency divergence detected"); err != nil {
			return nil, err
		}
	}

	return check, nil
}

// FreezeForReview marks a model as frozen, suspending all further processing.
func (s *Service) FreezeForReview(ctx context.Context, modelNo, reason string) error {
	subs, err := s.summaryRepo.FindByModel(ctx, modelNo)
	if err != nil {
		return err
	}
	for _, sum := range subs {
		if sum.Status == domain.SummaryFrozen || sum.Status == domain.SummaryFinal {
			continue
		}
		if err := s.sm.Transition(sum.Status, domain.SummaryFrozen); err != nil {
			s.logger.Warn(ctx, "cannot freeze summary", "summary_id", sum.ID, "current_status", sum.Status)
			continue
		}
		sum.Status = domain.SummaryFrozen
		sum.FrozenReason = reason
		sum.Audit.UpdatedAt = s.clk.Now()
		if err := s.summaryRepo.Save(ctx, sum); err != nil {
			return err
		}

		sub, found, err := s.submissionRepo.Get(ctx, sum.SubmissionID)
		if err == nil && found {
			if sub.Status.CanTransitionTo(domain.SubmissionFrozen) {
				sub.Status = domain.SubmissionFrozen
				sub.Audit.UpdatedAt = s.clk.Now()
				_ = s.submissionRepo.Save(ctx, sub)
			}
		}

		s.logger.Warn(ctx, "model frozen for review",
			"model_no", modelNo, "summary_id", sum.ID, "reason", reason)
	}
	return nil
}

// RepairStale finds stale summaries and recomputes them.
func (s *Service) RepairStale(ctx context.Context) (int, error) {
	all, _, err := s.summaryRepo.ListPaged(ctx, domain.PaginationParams{Page: 1, PageSize: 500})
	if err != nil {
		return 0, err
	}
	repaired := 0
	for _, sum := range all {
		if sum.Status != domain.SummaryStale {
			continue
		}
		if _, err := s.RecomputeSummary(ctx, sum.SubmissionID); err != nil {
			s.logger.Error(ctx, "repair failed", "summary_id", sum.ID, "error", err)
			continue
		}
		repaired++
	}
	return repaired, nil
}

// MarkStale marks a summary as stale when a detail result changes.
func (s *Service) MarkStale(ctx context.Context, submissionID string) error {
	summary, err := s.summaryRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return err
	}
	if summary == nil {
		return nil
	}
	if summary.Status == domain.SummaryFrozen {
		return nil
	}
	if err := s.sm.Transition(summary.Status, domain.SummaryStale); err != nil {
		return nil
	}
	summary.Status = domain.SummaryStale
	summary.Audit.UpdatedAt = s.clk.Now()
	return s.summaryRepo.Save(ctx, summary)
}

// GetSummary retrieves the summary for a submission.
func (s *Service) GetSummary(ctx context.Context, submissionID string) (*domain.Summary, error) {
	summary, err := s.summaryRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return nil, errorsx.ErrNotFound
	}
	return summary, nil
}

// ListSummaries returns paged summaries.
func (s *Service) ListSummaries(ctx context.Context, params domain.PaginationParams) ([]*domain.Summary, int, error) {
	return s.summaryRepo.ListPaged(ctx, params)
}
