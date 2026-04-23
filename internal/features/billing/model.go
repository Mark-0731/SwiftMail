package billing

import (
	"time"

	"github.com/google/uuid"
)

// Credit represents a user's email credit balance.
type Credit struct {
	UserID             uuid.UUID `json:"user_id"`
	Balance            int64     `json:"balance"`
	AutoTopupEnabled   bool      `json:"auto_topup_enabled"`
	AutoTopupThreshold int64     `json:"auto_topup_threshold"`
	AutoTopupAmount    int64     `json:"auto_topup_amount"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Usage represents email usage stats.
type Usage struct {
	UserID  uuid.UUID `json:"user_id"`
	Month   string    `json:"month"` // "2026-04"
	Sent    int64     `json:"sent"`
	Balance int64     `json:"balance"` // Current credit balance
}

// Pricing constants
const (
	MinimumTopUpUSD     = 10                                 // Minimum $10 top-up
	CreditsPerDollar    = 1000                               // $1 = 1,000 email credits
	MinimumTopUpCredits = MinimumTopUpUSD * CreditsPerDollar // 10,000 credits minimum
)

// CalculateCredits converts USD amount to email credits
// Formula: $1 = 1,000 emails
// Example: $10 = 10,000 emails, $50 = 50,000 emails
func CalculateCredits(amountUSD int64) int64 {
	return amountUSD * CreditsPerDollar
}

// CalculatePrice converts credits to USD amount
func CalculatePrice(credits int64) int64 {
	return credits / CreditsPerDollar
}
