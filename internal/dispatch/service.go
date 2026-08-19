package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/gateway"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// Service orchestrates dispatching certification tasks to testing agencies.
// It handles creation, assignment, execution, result receipt, timeout,
// failover, and retry — all with full state tracking and idempotency.
type Service struct {
	dispatchRepo   *repository.DispatchRepo
	submissionRepo *repository.SubmissionRepo
	agencyRepo     *repository.AgencyRepo
	resultRepo     *repository.ResultRepo
	idemRepo       *repository.IdempotencyRepo
	selector       *AgencySelector
	failover       *FailoverHandler
	gateway        *gateway.Manager
	logger         logging.Logger
	clk            clock.Clock
	dispatchSM     DispatchStateMachine
}

type DispatchStateMachine struct{}

func (dsm DispatchStateMachine) Transition(current, target domain.DispatchStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func NewService(
	dispatchRepo *repository.DispatchRepo,
	submissionRepo *repository.SubmissionRepo,
	agencyRepo *repository.AgencyRepo,
	resultRepo *repository.ResultRepo,
	idemRepo *repository.IdempotencyRepo,
	gw *gateway.Manager,
	logger logging.Logger,
	clk clock.Clock,
) *Service {
	selector := NewAgencySelector(agencyRepo, dispatchRepo)
	failover := NewFailoverHandler(selector, FailoverConfig{
		Timeout:     10 * time.Second,
		MaxRetries:  3,
		BaseBackoff: 500 * time.Millisecond,
		MaxBackoff:  10 * time.Second,
	}, clk.Now)
	return &Service{
		dispatchRepo:   dispatchRepo,
		submissionRepo: submissionRepo,
		agencyRepo:     agencyRepo,
		resultRepo:     resultRepo,
		idemRepo:       idemRepo,
		selector:       selector,
		failover:       failover,
		gateway:        gw,
		logger:         logger,
		clk:            clk,
		dispatchSM:     DispatchStateMachine{},
	}
}

// DispatchSubmission creates one dispatch task per standard for the submission.
// If tasks already exist for this submission (idempotent re-dispatch), no
// duplicates are created.
func (s *Service) DispatchSubmission(ctx context.Context, submissionID string) ([]*domain.DispatchTask, error) {
	sub, found, err := s.submissionRepo.Get(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}

	existing, err := s.dispatchRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && !allFailed(existing) {
		return existing, nil
	}

	var tasks []*domain.DispatchTask
	now := s.clk.Now()
	for _, stdCode := range sub.StandardCodes {
		agency, err := s.selector.SelectForStandard(ctx, stdCode)
		if err != nil {
			return nil, err
		}
		if agency == nil {
			return nil, fmt.Errorf("%w: no agency for standard %s", errorsx.ErrNoUpstream, stdCode)
		}
		task := &domain.DispatchTask{
			ID:           "dsp_" + uuid.NewString(),
			SubmissionID: submissionID,
			ModelNo:      sub.ModelNo,
			BatchNo:      sub.BatchNo,
			StandardCode: stdCode,
			AgencyID:     agency.ID,
			AgencyName:   agency.Name,
			Status:       domain.DispatchAssigned,
			Priority:     agency.Priority,
			Attempt:      1,
			MaxAttempts:  3,
			AssignedAt:   now,
			Audit: domain.AuditMeta{
				CreatedBy: "system",
				CreatedAt: now,
				UpdatedBy: "system",
				UpdatedAt: now,
			},
		}
		if err := s.dispatchRepo.Save(ctx, task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	sub.RecordDispatchRound()
	if err := s.transitionSubmission(ctx, sub, domain.SubmissionDispatched); err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "dispatched submission",
		"submission_id", submissionID,
		"task_count", len(tasks))
	return tasks, nil
}

func allFailed(tasks []*domain.DispatchTask) bool {
	for _, t := range tasks {
		if t.Status != domain.DispatchFailed && t.Status != domain.DispatchTimeout {
			return false
		}
	}
	return len(tasks) > 0
}

// ExecuteDispatch sends a task to the upstream gateway and processes the response.
func (s *Service) ExecuteDispatch(ctx context.Context, taskID string) error {
	task, found, err := s.dispatchRepo.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if !found {
		return errorsx.ErrNotFound
	}

	if err := s.dispatchSM.Transition(task.Status, domain.DispatchInProgress); err != nil {
		return err
	}
	task.Status = domain.DispatchInProgress
	task.Audit.UpdatedAt = s.clk.Now()
	if err := s.dispatchRepo.Save(ctx, task); err != nil {
		return err
	}

	reqBody, _ := json.Marshal(map[string]any{
		"dispatch_id":   task.ID,
		"submission_id": task.SubmissionID,
		"model_no":      task.ModelNo,
		"standard_code": task.StandardCode,
		"agency_id":     task.AgencyID,
	})

	result, err := s.gateway.CallWithFailover(ctx, "POST", "/api/v1/test", reqBody, task.ID)
	if err != nil {
		return s.handleDispatchFailure(ctx, task, err)
	}

	task.LastUpstreamID = result.UpstreamID
	if result.StatusCode >= 400 {
		return s.handleDispatchFailure(ctx, task, fmt.Errorf("upstream returned %d", result.StatusCode))
	}

	var upstreamResp struct {
		Passed     bool    `json:"passed"`
		Score      float64 `json:"score"`
		Conclusion string  `json:"conclusion"`
		Detail     string  `json:"detail"`
	}
	if err := json.Unmarshal(result.ResponseBody, &upstreamResp); err != nil {
		return s.handleDispatchFailure(ctx, task, err)
	}

	if err := s.dispatchSM.Transition(task.Status, domain.DispatchCompleted); err != nil {
		return err
	}
	now := s.clk.Now()
	task.Status = domain.DispatchCompleted
	task.CompletedAt = &now
	task.Audit.UpdatedAt = now
	if err := s.dispatchRepo.Save(ctx, task); err != nil {
		return err
	}

	testResult := &domain.TestResult{
		ID:           "res_" + uuid.NewString(),
		DispatchID:   task.ID,
		SubmissionID: task.SubmissionID,
		ModelNo:      task.ModelNo,
		StandardCode: task.StandardCode,
		AgencyID:     task.AgencyID,
		Passed:       upstreamResp.Passed,
		Score:        upstreamResp.Score,
		Conclusion:   upstreamResp.Conclusion,
		Detail:       upstreamResp.Detail,
		TestDate:     now,
		ReceivedAt:   now,
		Audit: domain.AuditMeta{
			CreatedBy: "system",
			CreatedAt: now,
			UpdatedBy: "system",
			UpdatedAt: now,
		},
	}
	if err := s.resultRepo.Save(ctx, testResult); err != nil {
		return err
	}

	s.logger.Info(ctx, "dispatch completed",
		"task_id", task.ID,
		"passed", upstreamResp.Passed,
		"score", upstreamResp.Score)
	return nil
}

func (s *Service) handleDispatchFailure(ctx context.Context, task *domain.DispatchTask, err error) error {
	task.Attempt++
	if task.Attempt >= task.MaxAttempts {
		task.Status = domain.DispatchDeadLetter
		task.FailoverReason = err.Error()
		nextRetry := s.failover.NextRetryAt(task.Attempt)
		task.NextRetryAt = &nextRetry
	} else {
		task.Status = domain.DispatchFailed
		task.FailoverReason = err.Error()
		nextRetry := s.failover.NextRetryAt(task.Attempt)
		task.NextRetryAt = &nextRetry
	}
	task.Audit.UpdatedAt = s.clk.Now()
	if saveErr := s.dispatchRepo.Save(ctx, task); saveErr != nil {
		return fmt.Errorf("save failed task: %w (original: %v)", saveErr, err)
	}
	return err
}

// HandleTimeout marks a task as timed out and triggers failover if possible.
func (s *Service) HandleTimeout(ctx context.Context, taskID string) error {
	task, found, err := s.dispatchRepo.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if !found {
		return errorsx.ErrNotFound
	}
	if err := s.dispatchSM.Transition(task.Status, domain.DispatchTimeout); err != nil {
		return err
	}
	task.Status = domain.DispatchTimeout
	task.Attempt++
	task.FailoverReason = "timeout"
	nextRetry := s.failover.NextRetryAt(task.Attempt)
	task.NextRetryAt = &nextRetry
	task.Audit.UpdatedAt = s.clk.Now()

	backup, err := s.failover.Failover(ctx, task, "timeout")
	if err != nil {
		s.logger.Warn(ctx, "failover failed, no backup available", "task_id", task.ID)
		if task.Attempt >= task.MaxAttempts {
			task.Status = domain.DispatchDeadLetter
		}
	}
	if backup != nil {
		task.AgencyID = backup.ID
		task.AgencyName = backup.Name
		s.logger.Info(ctx, "failover to backup agency",
			"task_id", task.ID,
			"new_agency", backup.ID)
	}
	return s.dispatchRepo.Save(ctx, task)
}

// RetryFailed finds failed/timeout tasks that are ready for retry and re-dispatches them.
func (s *Service) RetryFailed(ctx context.Context) (int, error) {
	tasks, err := s.dispatchRepo.FindByStatus(ctx, domain.DispatchFailed)
	if err != nil {
		return 0, err
	}
	retried := 0
	now := s.clk.Now()
	for _, task := range tasks {
		if task.NextRetryAt != nil && now.Before(*task.NextRetryAt) {
			continue
		}
		if err := s.dispatchSM.Transition(task.Status, domain.DispatchAssigned); err != nil {
			continue
		}
		task.Status = domain.DispatchAssigned
		task.Audit.UpdatedAt = now
		if err := s.dispatchRepo.Save(ctx, task); err != nil {
			continue
		}
		retried++
	}
	return retried, nil
}

func (s *Service) transitionSubmission(ctx context.Context, sub *domain.Submission, target domain.SubmissionStatus) error {
	if !sub.Status.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, sub.Status, target)
	}
	sub.Status = target
	sub.Audit.UpdatedAt = s.clk.Now()
	return s.submissionRepo.Save(ctx, sub)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.DispatchTask, error) {
	task, found, err := s.dispatchRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	return task, nil
}

func (s *Service) FindBySubmission(ctx context.Context, submissionID string) ([]*domain.DispatchTask, error) {
	return s.dispatchRepo.FindBySubmission(ctx, submissionID)
}

func (s *Service) ListPaged(ctx context.Context, params domain.PaginationParams, agencyFilter, statusFilter string) ([]*domain.DispatchTask, int, error) {
	return s.dispatchRepo.ListPaged(ctx, params, agencyFilter, statusFilter)
}

func (s *Service) FindPendingRetry(ctx context.Context) ([]*domain.DispatchTask, error) {
	var result []*domain.DispatchTask
	for _, status := range []domain.DispatchStatus{domain.DispatchFailed, domain.DispatchTimeout, domain.DispatchRetrying} {
		tasks, err := s.dispatchRepo.FindByStatus(ctx, status)
		if err != nil {
			return nil, err
		}
		result = append(result, tasks...)
	}
	return result, nil
}

func (s *Service) CountByStatus(ctx context.Context, status domain.DispatchStatus) (int, error) {
	tasks, err := s.dispatchRepo.FindByStatus(ctx, status)
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}
