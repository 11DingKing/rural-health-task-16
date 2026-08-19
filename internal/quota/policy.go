package quota

import (
	"time"

	"ruralhealth/internal/domain"
)

// BucketQuotaReservations is the KV bucket holding per-enterprise
// reservation records. Each record cross-references a submission by
// id, forming one of the persisted, cross-referenced record types.
const (
	BucketQuotaReservations = "quota_reservations"
	IndexEnterprise         = "enterprise"
	IndexSubmission         = "submission"
)

// Policy configures the per-enterprise submission quota.
type Policy struct {
	// MaxActivePerEnterprise is the maximum number of non-terminal
	// submissions an enterprise may hold concurrently. Acquiring beyond
	// this limit returns ErrQuotaExceeded.
	MaxActivePerEnterprise int
	// ReservationRetention bounds how long released reservations are
	// kept for audit before SweepExpired deletes them.
	ReservationRetention time.Duration
}

// DefaultPolicy returns production defaults.
func DefaultPolicy() Policy {
	return Policy{
		MaxActivePerEnterprise: 5,
		ReservationRetention:   24 * time.Hour,
	}
}

// Reservation records that an enterprise has acquired a quota slot for
// a submission. A reservation is "active" while ReleasedAt is nil.
type Reservation struct {
	ID             string           `json:"id"`
	EnterpriseID   string           `json:"enterprise_id"`
	SubmissionID   string           `json:"submission_id,omitempty"`
	ModelNo        string           `json:"model_no,omitempty"`
	AcquiredAt     time.Time        `json:"acquired_at"`
	ReleasedAt     *time.Time       `json:"released_at,omitempty"`
	ReleasedReason string           `json:"released_reason,omitempty"`
	Audit          domain.AuditMeta `json:"audit"`
}

// IsActive reports whether the reservation still consumes a quota slot.
func (r Reservation) IsActive() bool { return r.ReleasedAt == nil }

// Usage summarizes an enterprise's quota consumption.
type Usage struct {
	EnterpriseID string    `json:"enterprise_id"`
	ActiveCount  int       `json:"active_count"`
	MaxActive    int       `json:"max_active"`
	WindowEnd    time.Time `json:"window_end"`
}

// Remaining returns how many more submissions the enterprise may acquire.
func (u Usage) Remaining() int {
	rem := u.MaxActive - u.ActiveCount
	if rem < 0 {
		rem = 0
	}
	return rem
}
