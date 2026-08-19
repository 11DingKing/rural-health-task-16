package sla

import "time"

// Bucket and secondary-index names for SLA records.
const (
	BucketSLARecords = "sla_records"
	IndexDispatch    = "dispatch"
	IndexEscalation  = "escalation"
)

// Policy configures SLA deadlines and escalation timing.
type Policy struct {
	// DefaultDuration is the SLA granted to a dispatch when no
	// per-standard override exists.
	DefaultDuration time.Duration
	// WarningRatio is the fraction of the duration at which a dispatch
	// enters the warning state (0 < r < 1).
	WarningRatio float64
	// PerStandard overrides the default duration for specific standards.
	PerStandard map[string]time.Duration
	// EscalationGrace is how long after the deadline a breached dispatch
	// waits before ScanBreaches escalates it.
	EscalationGrace time.Duration
}

// DefaultPolicy returns production defaults.
func DefaultPolicy() Policy {
	return Policy{
		DefaultDuration: 30 * time.Minute,
		WarningRatio:    0.75,
		PerStandard:     map[string]time.Duration{},
		EscalationGrace: 0,
	}
}

// DurationFor returns the SLA duration for a standard code, falling back
// to the default when no override is configured.
func (p Policy) DurationFor(standardCode string) time.Duration {
	if d, ok := p.PerStandard[standardCode]; ok && d > 0 {
		return d
	}
	if p.DefaultDuration <= 0 {
		return 30 * time.Minute
	}
	return p.DefaultDuration
}

// WarningRatioSafe clamps the warning ratio to (0,1).
func (p Policy) WarningRatioSafe() float64 {
	r := p.WarningRatio
	if r <= 0 || r >= 1 {
		return 0.75
	}
	return r
}
