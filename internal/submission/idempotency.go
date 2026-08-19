package submission

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/errorsx"
	"ruralhealth/internal/repository"
)

type IdempotencyChecker struct {
	idempotencyRepo *repository.IdempotencyRepo
	submissionRepo  *repository.SubmissionRepo
	clock           func() time.Time
}

func NewIdempotencyChecker(
	idemRepo *repository.IdempotencyRepo,
	subRepo *repository.SubmissionRepo,
	clock func() time.Time,
) *IdempotencyChecker {
	return &IdempotencyChecker{
		idempotencyRepo: idemRepo,
		submissionRepo:  subRepo,
		clock:           clock,
	}
}

func (ic *IdempotencyChecker) CheckOrRecord(ctx context.Context, key, businessKey string) (*domain.Submission, error) {
	existing, found, err := ic.idempotencyRepo.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("check idempotency: %w", err)
	}
	if found && existing != nil {
		parts := strings.SplitN(businessKey, ":", 2)
		if len(parts) == 2 {
			subs, err := ic.submissionRepo.FindByModelBatch(ctx, parts[0], parts[1])
			if err != nil {
				return nil, err
			}
			if len(subs) > 0 {
				return subs[0], errorsx.ErrIdempotentReplay
			}
		}
		return nil, errorsx.ErrIdempotentReplay
	}

	rec := &domain.IdempotencyRecord{
		Key:         key,
		BusinessKey: businessKey,
		Outcome:     "created",
		CreatedAt:   ic.clock(),
		ExpiresAt:   ic.clock().Add(24 * time.Hour),
	}
	if err := ic.idempotencyRepo.Save(ctx, rec); err != nil {
		return nil, fmt.Errorf("save idempotency record: %w", err)
	}
	return nil, nil
}

func (ic *IdempotencyChecker) MarkCompleted(ctx context.Context, key, outcome, resultJSON string) error {
	rec, found, err := ic.idempotencyRepo.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return errorsx.ErrNotFound
	}
	rec.Outcome = outcome
	rec.ResultJSON = resultJSON
	return ic.idempotencyRepo.Save(ctx, rec)
}

func BusinessKey(modelNo, batchNo string) string {
	return modelNo + ":" + batchNo
}
