package billing

import (
	"time"

	"github.com/google/uuid"
)

// Plan represents a billing plan.
type Plan struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MonthlyLimit int    `json:"monthly_limit"` // 0 = unlimited
	PriceUSD     int    `json:"price_usd"`     // in cents, -1 = custom
	DedicatedIP  bool   `json:"dedicated_ip"`
}

// Credit represents a user's email credit balance.
type Credit struct {
	UserID             uuid.UUID `json:"user_id"`
	Balance            int64     `json:"balance"`
	AutoTopupEnabled   bool      `json:"auto_topup_enabled"`
	AutoTopupThreshold int64     `json:"auto_topup_threshold"`
	AutoTopupAmount    int64     `json:"auto_topup_amount"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Usage represents monthly email usage.
type Usage struct {
	UserID    uuid.UUID `json:"user_id"`
	Month     string    `json:"month"` // "2026-04"
	Sent      int64     `json:"sent"`
	Limit     int64     `json:"limit"`
	Remaining int64     `json:"remaining"`
}

// Subscription represents a user's subscription.
type Subscription struct {
	UserID               uuid.UUID `json:"user_id"`
	StripeSubscriptionID string    `json:"stripe_subscription_id"`
	PlanID               string    `json:"plan_id"`
	Status               string    `json:"status"` // active, canceled, past_due, trialing
	CurrentPeriodStart   time.Time `json:"current_period_start"`
	CurrentPeriodEnd     time.Time `json:"current_period_end"`
	CancelAtPeriodEnd    bool      `json:"cancel_at_period_end"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AvailablePlans returns the list of billing plans.
var AvailablePlans = []Plan{
	{ID: "free", Name: "Free", MonthlyLimit: 1000, PriceUSD: 0, DedicatedIP: false},
	{ID: "starter", Name: "Starter", MonthlyLimit: 50000, PriceUSD: 2500, DedicatedIP: false},
	{ID: "pro", Name: "Pro", MonthlyLimit: 500000, PriceUSD: 9900, DedicatedIP: true},
	{ID: "enterprise", Name: "Enterprise", MonthlyLimit: 0, PriceUSD: -1, DedicatedIP: true},
}
