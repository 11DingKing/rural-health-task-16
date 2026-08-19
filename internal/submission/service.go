package submission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/logging"
	"ruralhealth/internal/repository"
)

// Service orchestrates the full submission lifecycle: create, update,
// list, batch dispatch, export, and cancel. It coordinates idempotency,
// state transitions, and charge computation.
type Service struct {
	submissionRepo *repository.SubmissionRepo
	ruleRepo       *repository.RuleRepo
	idempotency    *IdempotencyChecker
	stateMachine   *StateMachine
	chargeCalc     *ChargeCalculator
	auditRepo      *repository.IdempotencyRepo
	logger         logging.Logger
	clock          clock.Clock
	createMu       sync.Mutex
}

type Logger = logging.Logger

func NewService(
	subRepo *repository.SubmissionRepo,
	ruleRepo *repository.RuleRepo,
	idemRepo *repository.IdempotencyRepo,
	logger logging.Logger,
	clk clock.Clock,
) *Service {
	return &Service{
		submissionRepo: subRepo,
		ruleRepo:       ruleRepo,
		idempotency:    NewIdempotencyChecker(idemRepo, subRepo, clk.Now),
		stateMachine:   NewStateMachine(),
		chargeCalc:     NewChargeCalculator(500.0),
		auditRepo:      idemRepo,
		logger:         logger,
		clock:          clk,
	}
}

// CreateRequest holds the input for creating a new submission.
type CreateRequest struct {
	EnterpriseID   string                `json:"enterprise_id"`
	EnterpriseName string                `json:"enterprise_name"`
	ModelNo        string                `json:"model_no"`
	ModelName      string                `json:"model_name"`
	Category       domain.DeviceCategory `json:"category"`
	RiskLevel      domain.RiskLevel      `json:"risk_level"`
	StandardCodes  []string              `json:"standard_codes"`
	BatchNo        string                `json:"batch_no"`
	Notes          string                `json:"notes"`
}

// Create handles a new certification submission with idempotency.
// If the same model+batch already exists, returns the original with
// IsDuplicate=true and no side effects.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*domain.Submission, bool, error) {
	if err := validateCreate(req); err != nil {
		return nil, false, err
	}

	businessKey := BusinessKey(req.ModelNo, req.BatchNo)
	idempotencyKey := "sub_" + businessKey

	s.createMu.Lock()
	defer s.createMu.Unlock()

	existing, err := s.idempotency.CheckOrRecord(ctx, idempotencyKey, businessKey)
	if err != nil {
		if err == errorsx.ErrIdempotentReplay {
			return existing, true, nil
		}
		return nil, false, fmt.Errorf("idempotency check: %w", err)
	}

	now := s.clock.Now()
	charge := s.chargeCalc.Compute(len(req.StandardCodes), false, now)

	sub := &domain.Submission{
		ID:             "sub_" + uuid.NewString(),
		EnterpriseID:   req.EnterpriseID,
		EnterpriseName: req.EnterpriseName,
		ModelNo:        req.ModelNo,
		ModelName:      req.ModelName,
		Category:       req.Category,
		RiskLevel:      req.RiskLevel,
		StandardCodes:  req.StandardCodes,
		BatchNo:        req.BatchNo,
		Status:         domain.SubmissionPending,
		BaseCharge:     charge.Amount,
		Notes:          req.Notes,
		IdempotencyKey: idempotencyKey,
		Audit: domain.AuditMeta{
			CreatedBy: req.EnterpriseID,
			CreatedAt: now,
			UpdatedBy: req.EnterpriseID,
			UpdatedAt: now,
		},
		Charge: domain.ChargeInfo{
			Amount:      charge.Amount,
			Currency:    "CNY",
			ItemCount:   charge.ItemCount,
			BilledAt:    charge.BilledAt,
			IsDuplicate: false,
		},
	}

	if err := s.submissionRepo.Save(ctx, sub); err != nil {
		return nil, false, fmt.Errorf("save submission: %w", err)
	}

	s.logger.Info(ctx, "submission created",
		"submission_id", sub.ID,
		"model_no", sub.ModelNo,
		"batch_no", sub.BatchNo,
		"standards", len(sub.StandardCodes))

	return sub, false, nil
}

// UpdateRequest holds fields that can be modified after creation.
type UpdateRequest struct {
	ModelName     *string
	RiskLevel     *domain.RiskLevel
	StandardCodes *[]string
	Notes         *string
	UpdatedBy     string
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*domain.Submission, error) {
	sub, found, err := s.submissionRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if sub.Status.IsTerminal() {
		return nil, fmt.Errorf("%w: submission is terminal", errorsx.ErrConflict)
	}

	if req.ModelName != nil {
		sub.ModelName = *req.ModelName
	}
	if req.RiskLevel != nil {
		sub.RiskLevel = *req.RiskLevel
	}
	if req.StandardCodes != nil {
		sub.StandardCodes = *req.StandardCodes
	}
	if req.Notes != nil {
		sub.Notes = *req.Notes
	}
	sub.Audit.UpdatedBy = req.UpdatedBy
	sub.Audit.UpdatedAt = s.clock.Now()

	if err := s.submissionRepo.Save(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Submission, error) {
	sub, found, err := s.submissionRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	return sub, nil
}

func (s *Service) List(ctx context.Context, params domain.PaginationParams, statusFilter, enterpriseFilter string) ([]*domain.Submission, int, error) {
	return s.submissionRepo.ListPaged(ctx, params, statusFilter, enterpriseFilter)
}

func (s *Service) Cancel(ctx context.Context, id, cancelledBy string) (*domain.Submission, error) {
	sub, found, err := s.submissionRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if err := s.stateMachine.Transition(sub.Status, domain.SubmissionCancelled); err != nil {
		return nil, err
	}
	sub.Status = domain.SubmissionCancelled
	sub.Audit.UpdatedBy = cancelledBy
	sub.Audit.UpdatedAt = s.clock.Now()
	if err := s.submissionRepo.Save(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// TransitionStatus applies an explicit status transition with validation.
func (s *Service) TransitionStatus(ctx context.Context, id string, target domain.SubmissionStatus, actor string) (*domain.Submission, error) {
	sub, found, err := s.submissionRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsx.ErrNotFound
	}
	if err := s.stateMachine.Transition(sub.Status, target); err != nil {
		return nil, err
	}
	sub.Status = target
	sub.Audit.UpdatedBy = actor
	sub.Audit.UpdatedAt = s.clock.Now()
	if err := s.submissionRepo.Save(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// ExportReconciliation produces a reconciliation report for all submissions.
func (s *Service) ExportReconciliation(ctx context.Context, agencyFilter, dateFrom, dateTo string) ([]ReconciliationRow, error) {
	params := domain.PaginationParams{Page: 1, PageSize: 1000}
	subs, _, err := s.submissionRepo.ListPaged(ctx, params, "", enterpriseFilter(agencyFilter))
	if err != nil {
		return nil, err
	}
	_ = dateFrom
	_ = dateTo
	_ = subs

	results := make([]ReconciliationRow, 0)
	for _, sub := range subs {
		charge := sub.TotalCharge()
		results = append(results, ReconciliationRow{
			SubmissionID:   sub.ID,
			EnterpriseName: sub.EnterpriseName,
			ModelNo:        sub.ModelNo,
			BatchNo:        sub.BatchNo,
			Status:         string(sub.Status),
			ChargeAmount:   charge.String(),
			IsDuplicate:    sub.Charge.IsDuplicate,
		})
	}
	return results, nil
}

type ReconciliationRow struct {
	SubmissionID   string `json:"submission_id"`
	EnterpriseName string `json:"enterprise_name"`
	ModelNo        string `json:"model_no"`
	BatchNo        string `json:"batch_no"`
	Status         string `json:"status"`
	ChargeAmount   string `json:"charge_amount"`
	IsDuplicate    bool   `json:"is_duplicate"`
}

func validateCreate(req CreateRequest) error {
	var me errorsx.MultiError
	if req.EnterpriseID == "" {
		me.Add("enterprise_id", "must not be empty")
	}
	if req.ModelNo == "" {
		me.Add("model_no", "must not be empty")
	}
	if req.ModelName == "" {
		me.Add("model_name", "must not be empty")
	}
	if req.BatchNo == "" {
		me.Add("batch_no", "must not be empty")
	}
	if len(req.StandardCodes) == 0 {
		me.Add("standard_codes", "must have at least one standard")
	}
	if req.Category == "" {
		me.Add("category", "must not be empty")
	}
	if me.HasErrors() {
		return fmt.Errorf("%w: %s", errorsx.ErrValidation, &me)
	}
	return nil
}

func enterpriseFilter(agencyFilter string) string {
	_ = agencyFilter
	return ""
}

func (s *Service) ToJSONString(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
