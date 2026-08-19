package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// Common value objects shared across domain entities.

type ID string

type AuditMeta struct {
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PageRequest struct {
	Page     int
	PageSize int
}

type PageResult[T any] struct {
	Items    []T
	Total    int
	Page     int
	PageSize int
}

type RiskLevel int

const (
	RiskLevelLow    RiskLevel = 1
	RiskLevelMedium RiskLevel = 2
	RiskLevelHigh   RiskLevel = 3
)

type DeviceCategory string

const (
	CategoryExoskeleton    DeviceCategory = "exoskeleton"
	CategoryProsthesis     DeviceCategory = "prosthesis"
	CategoryOrthosis       DeviceCategory = "orthosis"
	CategoryRehabilitation DeviceCategory = "rehabilitation_robot"
)

// ChargeInfo represents the billing computation for a submission.
// Duplicate submissions for the same model+batch are idempotent and
// produce only one actual charge.
type ChargeInfo struct {
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	ItemCount   int             `json:"item_count"`
	BilledAt    time.Time       `json:"billed_at"`
	IsDuplicate bool            `json:"is_duplicate"`
}

// ConsistencyCheck holds the result of comparing detail conclusions
// against the summary. When inconsistent the model is frozen.
type ConsistencyCheck struct {
	ModelID        string    `json:"model_id"`
	DetailCount    int       `json:"detail_count"`
	SummaryCount   int       `json:"summary_count"`
	Consistent     bool      `json:"consistent"`
	CheckedAt      time.Time `json:"checked_at"`
	DivergenceNote string    `json:"divergence_note,omitempty"`
}

type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

func (p PaginationParams) Normalize() PaginationParams {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 200 {
		p.PageSize = 20
	}
	return p
}

func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}
