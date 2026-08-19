package callback

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/config"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// Service delivers result notifications to enterprises via webhook.
// It supports HMAC signing, exponential backoff retry, and duplicate
// notification deduplication.
type Service struct {
	callbackRepo   *repository.CallbackRepo
	submissionRepo *repository.SubmissionRepo
	signer         *Signer
	httpClient     *http.Client
	logger         logging.Logger
	clk            clock.Clock
	sm             CallbackStateMachine
	config         config.CallbackConfig
	mu             sync.Mutex
	delivering     map[string]bool
}

type CallbackStateMachine struct{}

func (csm CallbackStateMachine) Transition(current, target domain.CallbackStatus) error {
	if current == target {
		return nil
	}
	if !current.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", errorsx.ErrInvalidTransition, current, target)
	}
	return nil
}

func NewService(
	callbackRepo *repository.CallbackRepo,
	submissionRepo *repository.SubmissionRepo,
	signer *Signer,
	logger logging.Logger,
	clk clock.Clock,
	cfg config.CallbackConfig,
) *Service {
	return &Service{
		callbackRepo:   callbackRepo,
		submissionRepo: submissionRepo,
		signer:         signer,
		httpClient:     &http.Client{Timeout: cfg.Timeout},
		logger:         logger,
		clk:            clk,
		sm:             CallbackStateMachine{},
		config:         cfg,
		delivering:     make(map[string]bool),
	}
}

// CreateCallback creates a new notification for a submission.
// If a callback already exists for the submission (dedup), no duplicate is created.
func (s *Service) CreateCallback(ctx context.Context, submissionID, webhookURL string, summary any) (*domain.Callback, error) {
	existing, err := s.callbackRepo.FindBySubmission(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return existing[0], nil
	}

	payload, signature, err := s.signer.BuildPayload(submissionID, summary, s.clk.Now())
	if err != nil {
		return nil, err
	}

	now := s.clk.Now()
	cb := &domain.Callback{
		ID:           "cb_" + uuid.NewString(),
		SubmissionID: submissionID,
		WebhookURL:   webhookURL,
		Payload:      payload,
		Signature:    signature,
		Status:       domain.CallbackPending,
		Attempt:      0,
		MaxAttempts:  s.config.MaxAttempts,
		Audit: domain.AuditMeta{
			CreatedBy: "system",
			CreatedAt: now,
			UpdatedBy: "system",
			UpdatedAt: now,
		},
	}
	if err := s.callbackRepo.Save(ctx, cb); err != nil {
		return nil, err
	}
	s.logger.Info(ctx, "callback created", "callback_id", cb.ID, "submission_id", submissionID)
	return cb, nil
}

// DeliverCallback attempts to deliver a notification. Returns nil on success.
func (s *Service) DeliverCallback(ctx context.Context, callbackID string) error {
	s.mu.Lock()
	if s.delivering[callbackID] {
		s.mu.Unlock()
		return nil
	}
	s.delivering[callbackID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.delivering, callbackID)
		s.mu.Unlock()
	}()

	cb, found, err := s.callbackRepo.Get(ctx, callbackID)
	if err != nil {
		return err
	}
	if !found {
		return errorsx.ErrNotFound
	}
	if cb.Status == domain.CallbackConfirmed || cb.Status == domain.CallbackDelivered {
		return nil
	}

	if err := s.sm.Transition(cb.Status, domain.CallbackDelivering); err != nil {
		return err
	}
	cb.Status = domain.CallbackDelivering
	cb.Attempt++
	cb.Audit.UpdatedAt = s.clk.Now()
	if err := s.callbackRepo.Save(ctx, cb); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cb.WebhookURL, bytes.NewReader([]byte(cb.Payload)))
	if err != nil {
		return s.handleDeliveryFailure(ctx, cb, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", cb.Signature)
	req.Header.Set("X-Callback-ID", cb.ID)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.handleDeliveryFailure(ctx, cb, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		now := s.clk.Now()
		if err := s.sm.Transition(cb.Status, domain.CallbackDelivered); err != nil {
			return err
		}
		cb.Status = domain.CallbackDelivered
		cb.DeliveredAt = &now
		cb.Audit.UpdatedAt = now
		if err := s.callbackRepo.Save(ctx, cb); err != nil {
			return err
		}
		s.logger.Info(ctx, "callback delivered", "callback_id", cb.ID, "attempt", cb.Attempt)
		return nil
	}

	return s.handleDeliveryFailure(ctx, cb, fmt.Errorf("webhook returned %d", resp.StatusCode))
}

func (s *Service) handleDeliveryFailure(ctx context.Context, cb *domain.Callback, err error) error {
	cb.LastError = err.Error()
	if cb.Attempt >= cb.MaxAttempts {
		if err := s.sm.Transition(cb.Status, domain.CallbackDead); err == nil {
			cb.Status = domain.CallbackDead
		} else {
			cb.Status = domain.CallbackFailed
		}
	} else {
		if err := s.sm.Transition(cb.Status, domain.CallbackPending); err == nil {
			cb.Status = domain.CallbackPending
		} else {
			cb.Status = domain.CallbackFailed
		}
		nextRetry := s.nextRetryAt(cb.Attempt)
		cb.NextRetryAt = &nextRetry
	}
	cb.Audit.UpdatedAt = s.clk.Now()
	_ = s.callbackRepo.Save(ctx, cb)
	s.logger.Warn(ctx, "callback delivery failed",
		"callback_id", cb.ID, "attempt", cb.Attempt, "error", err)
	return err
}

func (s *Service) nextRetryAt(attempt int) time.Time {
	backoff := s.config.BaseBackoff
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > s.config.MaxBackoff {
			backoff = s.config.MaxBackoff
			break
		}
	}
	return s.clk.Now().Add(backoff)
}

// ConfirmCallback marks a callback as confirmed by the enterprise.
func (s *Service) ConfirmCallback(ctx context.Context, callbackID string) (*domain.Callback, error) {
	cb, found, err := s.callbackRepo.Get(ctx, callbackID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if err := s.sm.Transition(cb.Status, domain.CallbackConfirmed); err != nil {
		return nil, err
	}
	now := s.clk.Now()
	cb.Status = domain.CallbackConfirmed
	cb.ConfirmedAt = &now
	cb.Audit.UpdatedAt = now
	if err := s.callbackRepo.Save(ctx, cb); err != nil {
		return nil, err
	}
	return cb, nil
}

// RetryPending finds pending callbacks ready for retry and delivers them.
func (s *Service) RetryPending(ctx context.Context) (int, error) {
	pending, err := s.callbackRepo.FindByStatus(ctx, domain.CallbackPending)
	if err != nil {
		return 0, err
	}
	now := s.clk.Now()
	delivered := 0
	for _, cb := range pending {
		if cb.NextRetryAt != nil && now.Before(*cb.NextRetryAt) {
			continue
		}
		if err := s.DeliverCallback(ctx, cb.ID); err != nil {
			s.logger.Warn(ctx, "retry callback failed", "callback_id", cb.ID)
			continue
		}
		delivered++
	}
	return delivered, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Callback, error) {
	cb, found, err := s.callbackRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	return cb, nil
}

func (s *Service) FindBySubmission(ctx context.Context, submissionID string) ([]*domain.Callback, error) {
	return s.callbackRepo.FindBySubmission(ctx, submissionID)
}
