package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stripe/stripe-go/v85"
)

// Service manages billing credits, subscriptions, and Stripe integration.
type Service struct {
	db     *pgxpool.Pool
	rdb    *redis.Client
	stripe *StripeService
	logger zerolog.Logger
}

// NewService creates a billing service.
func NewService(db *pgxpool.Pool, rdb *redis.Client, stripe *StripeService, logger zerolog.Logger) *Service {
	return &Service{
		db:     db,
		rdb:    rdb,
		stripe: stripe,
		logger: logger,
	}
}

// GetCredits returns the current credit balance for a user.
func (s *Service) GetCredits(ctx context.Context, userID uuid.UUID) (*Credit, error) {
	c := &Credit{}
	err := s.db.QueryRow(ctx,
		`SELECT user_id, balance, auto_topup_enabled, auto_topup_threshold, auto_topup_amount, updated_at 
		 FROM credits WHERE user_id = $1`, userID,
	).Scan(&c.UserID, &c.Balance, &c.AutoTopupEnabled, &c.AutoTopupThreshold, &c.AutoTopupAmount, &c.UpdatedAt)

	if err != nil {
		// Create default credit record
		_, err = s.db.Exec(ctx,
			`INSERT INTO credits (user_id, balance) VALUES ($1, 1000) ON CONFLICT (user_id) DO NOTHING`, userID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create credits: %w", err)
		}
		return &Credit{UserID: userID, Balance: 1000, UpdatedAt: time.Now()}, nil
	}
	return c, nil
}

// DeductCredit deducts 1 credit atomically. Returns error if insufficient.
func (s *Service) DeductCredit(ctx context.Context, userID uuid.UUID) error {
	result, err := s.db.Exec(ctx,
		`UPDATE credits SET balance = balance - 1, updated_at = NOW() WHERE user_id = $1 AND balance > 0`, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("insufficient credits")
	}

	// Check if auto-topup is needed
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error().Interface("panic", r).Msg("panic in auto-topup goroutine")
			}
		}()
		s.checkAutoTopup(context.Background(), userID)
	}()

	return nil
}

// AddCredits adds credits to a user's balance.
func (s *Service) AddCredits(ctx context.Context, userID uuid.UUID, amount int64, txType, description, stripePaymentID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update balance
	_, err = tx.Exec(ctx,
		`UPDATE credits SET balance = balance + $1, updated_at = NOW() WHERE user_id = $2`, amount, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update credits: %w", err)
	}

	// Record transaction
	_, err = tx.Exec(ctx,
		`INSERT INTO credit_transactions (user_id, amount, type, description, stripe_payment_id) 
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, amount, txType, description, stripePaymentID,
	)
	if err != nil {
		return fmt.Errorf("failed to record transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate Redis cache
	s.rdb.Del(ctx, fmt.Sprintf("credits:%s", userID.String()))

	s.logger.Info().Str("user_id", userID.String()).Int64("amount", amount).Str("type", txType).Msg("credits added")
	return nil
}

// HasCredits checks if a user has available credits (Redis-first for speed).
func (s *Service) HasCredits(ctx context.Context, userID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("credits:%s", userID.String())

	// Try Redis first
	balance, err := s.rdb.Get(ctx, key).Int64()
	if err == nil {
		return balance > 0, nil
	}

	// Fallback to DB
	credits, err := s.GetCredits(ctx, userID)
	if err != nil {
		return false, err
	}

	// Cache in Redis (5 min TTL)
	s.rdb.Set(ctx, key, credits.Balance, 5*time.Minute)

	return credits.Balance > 0, nil
}

// GetUsage returns monthly usage stats.
func (s *Service) GetUsage(ctx context.Context, userID uuid.UUID) (*Usage, error) {
	month := time.Now().Format("2006-01")

	// Count sent this month from email_logs
	var sent int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_logs WHERE user_id = $1 AND created_at >= date_trunc('month', NOW())`, userID,
	).Scan(&sent)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}

	// Get subscription info
	sub, err := s.GetSubscription(ctx, userID)
	var limit int64 = 1000 // Free plan default
	if err == nil && sub != nil {
		plan := findPlan(sub.PlanID)
		limit = int64(plan.MonthlyLimit)
	}

	remaining := limit - sent
	if remaining < 0 {
		remaining = 0
	}

	return &Usage{
		UserID:    userID,
		Month:     month,
		Sent:      sent,
		Limit:     limit,
		Remaining: remaining,
	}, nil
}

// CreateCheckoutSession creates a Stripe checkout session for subscription.
func (s *Service) CreateCheckoutSession(ctx context.Context, userID uuid.UUID, planID string) (*stripe.CheckoutSession, error) {
	// Get or create Stripe customer
	var customerID string
	err := s.db.QueryRow(ctx,
		`SELECT stripe_customer_id FROM users WHERE id = $1`, userID,
	).Scan(&customerID)

	if err != nil || customerID == "" {
		// Get user email and name
		var email, name string
		err = s.db.QueryRow(ctx,
			`SELECT email, name FROM users WHERE id = $1`, userID,
		).Scan(&email, &name)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		// Create Stripe customer
		customerID, err = s.stripe.CreateCustomer(ctx, userID, email, name)
		if err != nil {
			return nil, err
		}

		// Save customer ID
		_, err = s.db.Exec(ctx,
			`UPDATE users SET stripe_customer_id = $1 WHERE id = $2`, customerID, userID,
		)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to save stripe customer ID")
		}
	}

	// Get price ID for plan
	priceID := s.stripe.GetPriceID(planID)
	if priceID == "" {
		return nil, fmt.Errorf("invalid plan ID: %s", planID)
	}

	// Create checkout session
	return s.stripe.CreateCheckoutSession(ctx, customerID, priceID, userID)
}

// CreatePaymentIntent creates a payment intent for credit purchase.
func (s *Service) CreatePaymentIntent(ctx context.Context, userID uuid.UUID, amount int64) (*stripe.PaymentIntent, error) {
	// Get or create Stripe customer
	var customerID string
	err := s.db.QueryRow(ctx,
		`SELECT stripe_customer_id FROM users WHERE id = $1`, userID,
	).Scan(&customerID)

	if err != nil || customerID == "" {
		var email, name string
		err = s.db.QueryRow(ctx,
			`SELECT email, name FROM users WHERE id = $1`, userID,
		).Scan(&email, &name)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		customerID, err = s.stripe.CreateCustomer(ctx, userID, email, name)
		if err != nil {
			return nil, err
		}

		_, err = s.db.Exec(ctx,
			`UPDATE users SET stripe_customer_id = $1 WHERE id = $2`, customerID, userID,
		)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to save stripe customer ID")
		}
	}

	return s.stripe.CreatePaymentIntent(ctx, customerID, amount, userID)
}

// GetSubscription returns the user's current subscription.
func (s *Service) GetSubscription(ctx context.Context, userID uuid.UUID) (*Subscription, error) {
	sub := &Subscription{}
	err := s.db.QueryRow(ctx,
		`SELECT user_id, stripe_subscription_id, plan_id, status, current_period_start, current_period_end, cancel_at_period_end, created_at, updated_at
		 FROM subscriptions WHERE user_id = $1 AND status IN ('active', 'trialing')`, userID,
	).Scan(&sub.UserID, &sub.StripeSubscriptionID, &sub.PlanID, &sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.CancelAtPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt)

	if err != nil {
		return nil, nil // No active subscription
	}
	return sub, nil
}

// CancelSubscription cancels a user's subscription.
func (s *Service) CancelSubscription(ctx context.Context, userID uuid.UUID) error {
	sub, err := s.GetSubscription(ctx, userID)
	if err != nil || sub == nil {
		return fmt.Errorf("no active subscription found")
	}

	// Cancel in Stripe
	err = s.stripe.CancelSubscription(ctx, sub.StripeSubscriptionID)
	if err != nil {
		return err
	}

	// Update in database
	_, err = s.db.Exec(ctx,
		`UPDATE subscriptions SET cancel_at_period_end = true, updated_at = NOW() WHERE user_id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Msg("subscription cancelled")
	return nil
}

// HandleWebhook processes Stripe webhook events.
func (s *Service) HandleWebhook(ctx context.Context, event stripe.Event) error {
	s.logger.Info().Str("type", string(event.Type)).Msg("processing Stripe webhook")

	switch event.Type {
	case "checkout.session.completed":
		return s.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.created", "customer.subscription.updated":
		return s.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return s.handleSubscriptionDeleted(ctx, event)
	case "payment_intent.succeeded":
		return s.handlePaymentSucceeded(ctx, event)
	case "payment_intent.payment_failed":
		return s.handlePaymentFailed(ctx, event)
	default:
		s.logger.Debug().Str("type", string(event.Type)).Msg("unhandled webhook event")
	}

	return nil
}

func (s *Service) handleCheckoutCompleted(ctx context.Context, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	userID, err := uuid.Parse(session.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Str("session_id", session.ID).Msg("checkout completed")
	return nil
}

func (s *Service) handleSubscriptionUpdated(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	userID, err := uuid.Parse(sub.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	// Determine plan ID from price
	planID := "free"
	if len(sub.Items.Data) > 0 {
		priceID := sub.Items.Data[0].Price.ID
		if priceID == s.stripe.priceIDStarter {
			planID = "starter"
		} else if priceID == s.stripe.priceIDPro {
			planID = "pro"
		}
	}

	// Extract actual period from Stripe subscription items
	now := time.Now()
	periodEnd := now.AddDate(0, 1, 0) // Default to 1 month from now

	if len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		// Use the price's recurring interval to calculate period end
		interval := sub.Items.Data[0].Price.Recurring.Interval
		intervalCount := sub.Items.Data[0].Price.Recurring.IntervalCount

		switch interval {
		case "month":
			periodEnd = now.AddDate(0, int(intervalCount), 0)
		case "year":
			periodEnd = now.AddDate(int(intervalCount), 0, 0)
		case "week":
			periodEnd = now.AddDate(0, 0, int(intervalCount)*7)
		case "day":
			periodEnd = now.AddDate(0, 0, int(intervalCount))
		}
	}

	// Upsert subscription
	_, err = s.db.Exec(ctx,
		`INSERT INTO subscriptions (user_id, stripe_subscription_id, plan_id, status, current_period_start, current_period_end, cancel_at_period_end)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id) DO UPDATE SET
		 stripe_subscription_id = $2, plan_id = $3, status = $4, current_period_start = $5, current_period_end = $6, cancel_at_period_end = $7, updated_at = NOW()`,
		userID, sub.ID, planID, string(sub.Status),
		now, periodEnd, sub.CancelAtPeriodEnd,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert subscription: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Str("plan_id", planID).Msg("subscription updated")
	return nil
}

func (s *Service) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	userID, err := uuid.Parse(sub.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	// Update subscription status
	_, err = s.db.Exec(ctx,
		`UPDATE subscriptions SET status = 'canceled', updated_at = NOW() WHERE user_id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Msg("subscription deleted")
	return nil
}

func (s *Service) handlePaymentSucceeded(ctx context.Context, event stripe.Event) error {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return fmt.Errorf("failed to unmarshal payment intent: %w", err)
	}

	userID, err := uuid.Parse(pi.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	// Add credits (1 credit per cent)
	credits := pi.Amount
	err = s.AddCredits(ctx, userID, credits, "purchase", fmt.Sprintf("Credit purchase via Stripe"), pi.ID)
	if err != nil {
		return fmt.Errorf("failed to add credits: %w", err)
	}

	s.logger.Info().Str("user_id", userID.String()).Int64("credits", credits).Msg("payment succeeded, credits added")
	return nil
}

func (s *Service) handlePaymentFailed(ctx context.Context, event stripe.Event) error {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return fmt.Errorf("failed to unmarshal payment intent: %w", err)
	}

	userID, err := uuid.Parse(pi.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	s.logger.Warn().Str("user_id", userID.String()).Str("payment_intent_id", pi.ID).Msg("payment failed")
	return nil
}

func (s *Service) checkAutoTopup(ctx context.Context, userID uuid.UUID) {
	credits, err := s.GetCredits(ctx, userID)
	if err != nil {
		return
	}

	if credits.AutoTopupEnabled && credits.Balance <= credits.AutoTopupThreshold {
		s.logger.Info().Str("user_id", userID.String()).Int64("balance", credits.Balance).Msg("auto-topup triggered")
		// Create payment intent for auto-topup
		_, err := s.CreatePaymentIntent(ctx, userID, credits.AutoTopupAmount*100) // Convert to cents
		if err != nil {
			s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("auto-topup failed")
		}
	}
}

func findPlan(id string) Plan {
	for _, p := range AvailablePlans {
		if p.ID == id {
			return p
		}
	}
	return AvailablePlans[0] // default to free
}
