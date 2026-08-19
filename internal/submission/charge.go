package submission

import (
	"time"

	"github.com/shopspring/decimal"
)

// ChargeCalculator computes billing for a submission. Duplicate
// submissions (same model+batch) are charged zero.
type ChargeCalculator struct {
	basePerStandard decimal.Decimal
}

func NewChargeCalculator(basePerStandard float64) *ChargeCalculator {
	return &ChargeCalculator{
		basePerStandard: decimal.NewFromFloat(basePerStandard),
	}
}

// Compute returns the charge for a submission based on the number of
// standards. Duplicates are charged zero.
func (cc *ChargeCalculator) Compute(standardCount int, isDuplicate bool, now time.Time) ChargeResult {
	if isDuplicate {
		return ChargeResult{
			Amount:      decimal.Zero,
			IsDuplicate: true,
			ItemCount:   0,
			BilledAt:    now,
		}
	}
	amount := cc.basePerStandard.Mul(decimal.NewFromInt(int64(standardCount)))
	return ChargeResult{
		Amount:      amount,
		IsDuplicate: false,
		ItemCount:   standardCount,
		BilledAt:    now,
	}
}

type ChargeResult struct {
	Amount      decimal.Decimal
	IsDuplicate bool
	ItemCount   int
	BilledAt    time.Time
}

// FormatAmount returns a human-readable charge string.
func (c ChargeResult) FormatAmount() string {
	return c.Amount.String() + " CNY"
}
