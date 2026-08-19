package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ruralhealth/internal/clock"
	"ruralhealth/internal/logging"
)

// Service provides audit logging: who did what, when, and to which entity.
type Service struct {
	store  *Store
	logger logging.Logger
	clk    clock.Clock
}

func NewService(store *Store, logger logging.Logger, clk clock.Clock) *Service {
	return &Service{store: store, logger: logger, clk: clk}
}

// Record creates an audit log entry for a business action.
func (s *Service) Record(ctx context.Context, entityType, entityID, action, actor, detail string) error {
	entry := AuditEntry{
		ID:         "aud_" + uuid.NewString(),
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Actor:      actor,
		Detail:     detail,
		OccurredAt: s.clk.Now(),
	}
	if err := s.store.Insert(ctx, entry); err != nil {
		s.logger.Error(ctx, "failed to write audit log", "error", err, "entity_type", entityType, "entity_id", entityID)
		return err
	}
	return nil
}

// FindByEntity returns all audit entries for a specific entity.
func (s *Service) FindByEntity(ctx context.Context, entityType, entityID string) ([]AuditEntry, error) {
	return s.store.FindByEntity(ctx, entityType, entityID)
}

// FindByActor returns all audit entries by a specific actor.
func (s *Service) FindByActor(ctx context.Context, actor string) ([]AuditEntry, error) {
	return s.store.FindByActor(ctx, actor)
}

// ListPaged returns paginated audit entries.
func (s *Service) ListPaged(ctx context.Context, page, pageSize int) ([]AuditEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.store.ListPaged(ctx, offset, pageSize)
}

// Ping checks if the audit database is reachable.
func (s *Service) Ping(ctx context.Context) error {
	return s.store.PingContext(ctx)
}

// Close closes the audit database.
func (s *Service) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

// AuditContext provides a fluent API for building audit entries.
type AuditContext struct {
	service    *Service
	entityType string
	entityID   string
	actor      string
}

func (s *Service) For(entityType, entityID, actor string) *AuditContext {
	return &AuditContext{
		service:    s,
		entityType: entityType,
		entityID:   entityID,
		actor:      actor,
	}
}

func (ac *AuditContext) Action(ctx context.Context, action string, detail string) error {
	return ac.service.Record(ctx, ac.entityType, ac.entityID, action, ac.actor, detail)
}

// FormatTimeRange returns a human-readable time range string.
func FormatTimeRange(from, to time.Time) string {
	return fmt.Sprintf("%s to %s", from.Format(time.RFC3339), to.Format(time.RFC3339))
}
