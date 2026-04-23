package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stripe/stripe-go/v85"

	"github.com/Mark-0731/SwiftMail/internal/features/billing"
	"github.com/Mark-0731/SwiftMail/internal/features/billing/infrastructure"
	"github.com/Mark-0731/SwiftMail/pkg/database"
)

// Service manages billing credits, and Stripe integration.
type Service struct {
	db     database.Querier
	rdb    *redis.Client
	stripe *infrastructure.StripeService
	logger zerolog.Logger
}

// NewService creates a billing service.
func NewService(db database.Querier, rdb *redis.Client, stripe *infrastructure.StripeService, logger zerolog.Logger) *Service {
	return &Service{
		db:     db,
		rdb:    rdb,
		stripe: stripe,
		logger: logger,
	}
}

// GetCredits returns the current credit balance for a user.
func (s *Service) GetCredits(ctx context.Context, userID uuid.UUID) (*billing.Credit, error) {
	c := &billing.Credit{}
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

		// Sync to Redis
		key := fmt.Sprintf("credits:%s", userID.String())
		s.rdb.Set(ctx, key, 1000, 0)

		return &billing.Credit{UserID: userID, Balance: 1000, UpdatedAt: time.Now()}, nil
	}

	// Sync to Redis cache whenever we read from DB
	key := fmt.Sprintf("credits:%s", userID.String())
	s.rdb.Set(ctx, key, c.Balance, 0)

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

	// Sync to Redis cache - get updated balance and set it
	credits, err := s.GetCredits(ctx, userID)
	if err == nil {
		key := fmt.Sprintf("credits:%s", userID.String())
		s.rdb.Set(ctx, key, credits.Balance, 0) // No expiry - permanent cache
		s.logger.Debug().Str("user_id", userID.String()).Int64("balance", credits.Balance).Msg("synced credits to Redis")
	}

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
func (s *Service) GetUsage(ctx context.Context, userID uuid.UUID) (*billing.Usage, error) {
	month := time.Now().Format("2006-01")

	// Count sent this month from email_logs
	var sent int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_logs WHERE user_id = $1 AND created_at >= date_trunc('month', NOW())`, userID,
	).Scan(&sent)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}

	// Get current credit balance
	credits, err := s.GetCredits(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get credits: %w", err)
	}

	return &billing.Usage{
		UserID:  userID,
		Month:   month,
		Sent:    sent,
		Balance: credits.Balance,
	}, nil
}

// GetStripePublishableKey returns the Stripe publishable key.
func (s *Service) GetStripePublishableKey() string {
	return s.stripe.GetPublishableKey()
}

// User can pay any amount ≥ $10, gets credits based on formula: $1 = 1,000 emails
func (s *Service) CreatePaymentIntent(ctx context.Context, userID uuid.UUID, amountUSD int64) (*stripe.PaymentIntent, error) {
	// Validate minimum amount
	if amountUSD < billing.MinimumTopUpUSD {
		return nil, fmt.Errorf("minimum top-up amount is $%d", billing.MinimumTopUpUSD)
	}

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

	// Calculate credits user will receive
	credits := billing.CalculateCredits(amountUSD)

	return s.stripe.CreatePaymentIntent(ctx, customerID, amountUSD, credits, userID)
}

func (s *Service) checkAutoTopup(ctx context.Context, userID uuid.UUID) {
	credits, err := s.GetCredits(ctx, userID)
	if err != nil {
		return
	}

	if credits.AutoTopupEnabled && credits.Balance <= credits.AutoTopupThreshold {
		s.logger.Info().Str("user_id", userID.String()).Int64("balance", credits.Balance).Msg("auto-topup triggered")
		// Create payment intent for auto-topup
		_, err := s.CreatePaymentIntent(ctx, userID, credits.AutoTopupAmount)
		if err != nil {
			s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("auto-topup failed")
		}
	}
}

// ConfirmPaymentAndAddCredits confirms a payment and immediately adds credits
// This is called after Stripe confirms the payment on the frontend
func (s *Service) ConfirmPaymentAndAddCredits(ctx context.Context, paymentIntentID string) error {
	// Get payment intent from Stripe to verify it succeeded
	pi, err := s.stripe.GetPaymentIntent(ctx, paymentIntentID)
	if err != nil {
		return fmt.Errorf("failed to get payment intent: %w", err)
	}

	// Check if payment succeeded
	if pi.Status != stripe.PaymentIntentStatusSucceeded {
		return fmt.Errorf("payment not succeeded, status: %s", pi.Status)
	}

	// Get user ID from metadata
	userID, err := uuid.Parse(pi.Metadata["user_id"])
	if err != nil {
		return fmt.Errorf("invalid user_id in metadata: %w", err)
	}

	// Get credits from metadata
	creditsStr, ok := pi.Metadata["credits"]
	if !ok {
		// Fallback: calculate from amount (old behavior)
		creditsStr = fmt.Sprintf("%d", pi.Amount)
	}

	var credits int64
	fmt.Sscanf(creditsStr, "%d", &credits)

	// Add credits
	err = s.AddCredits(ctx, userID, credits, "purchase", fmt.Sprintf("Top-up: $%.2f", float64(pi.Amount)/100), pi.ID)
	if err != nil {
		return fmt.Errorf("failed to add credits: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Int64("credits", credits).
		Int64("amount_cents", pi.Amount).
		Msg("payment confirmed, credits added")

	return nil
}

// GetTransactions returns credit transaction history.
func (s *Service) GetTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, amount, type, description, stripe_payment_id, created_at
		 FROM credit_transactions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	defer rows.Close()

	var transactions []map[string]interface{}
	for rows.Next() {
		var id, userIDStr, txType, description string
		var amount int64
		var stripePaymentID *string
		var createdAt time.Time

		if err := rows.Scan(&id, &userIDStr, &amount, &txType, &description, &stripePaymentID, &createdAt); err != nil {
			continue
		}

		tx := map[string]interface{}{
			"id":          id,
			"user_id":     userIDStr,
			"amount":      amount,
			"type":        txType,
			"description": description,
			"created_at":  createdAt,
		}
		if stripePaymentID != nil {
			tx["stripe_payment_id"] = *stripePaymentID
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}
