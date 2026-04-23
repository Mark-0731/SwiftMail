package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/platform/cache"
	"github.com/Mark-0731/SwiftMail/pkg/database"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// CreditService handles credit operations with proper rollback support.
type CreditService struct {
	cache  cache.Cache
	db     database.Querier
	logger zerolog.Logger
}

// NewCreditService creates a new credit service.
func NewCreditService(cache cache.Cache, db database.Querier, logger zerolog.Logger) *CreditService {
	return &CreditService{
		cache:  cache,
		db:     db,
		logger: logger,
	}
}

// DeductCredit deducts credits after successful email send.
func (cs *CreditService) DeductCredit(ctx context.Context, userID uuid.UUID, amount int64) error {
	remaining, err := cs.cache.DeductCredits(ctx, userID, amount)
	if err != nil {
		cs.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to deduct credits")
		return fmt.Errorf("failed to deduct credits: %w", err)
	}

	cs.logger.Info().
		Str("user_id", userID.String()).
		Int64("amount", amount).
		Int64("remaining", remaining).
		Msg("credits deducted successfully")

	return nil
}

// RefundCredit refunds credits if email send fails.
func (cs *CreditService) RefundCredit(ctx context.Context, userID uuid.UUID, amount int64, reason string) error {
	newBalance, err := cs.cache.AddCredits(ctx, userID, amount)
	if err != nil {
		cs.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to refund credits")
		return fmt.Errorf("failed to refund credits: %w", err)
	}

	// Log the refund for audit purposes
	refundKey := fmt.Sprintf("refund:%s:%d", userID.String(), time.Now().Unix())
	refundData := fmt.Sprintf(`{"user_id":"%s","amount":%d,"reason":"%s","timestamp":%d,"new_balance":%d}`,
		userID.String(), amount, reason, time.Now().Unix(), newBalance)

	cs.cache.SetWithExpiry(ctx, refundKey, refundData, 30*24*time.Hour)

	cs.logger.Info().
		Str("user_id", userID.String()).
		Int64("amount", amount).
		Str("reason", reason).
		Int64("new_balance", newBalance).
		Msg("credits refunded successfully")

	return nil
}

// CheckAndReserve atomically checks and reserves credits (prevents race condition).
func (cs *CreditService) CheckAndReserve(ctx context.Context, userID uuid.UUID, amount int64) (bool, int64, error) {
	reserved, newBalance, err := cs.cache.CheckAndReserveCredits(ctx, userID, amount)
	if err != nil {
		cs.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to check and reserve credits")
		return false, 0, fmt.Errorf("failed to check and reserve credits: %w", err)
	}

	if !reserved {
		cs.logger.Warn().
			Str("user_id", userID.String()).
			Int64("amount", amount).
			Int64("balance", newBalance).
			Msg("insufficient credits for reservation")
		return false, newBalance, nil
	}

	// Also update PostgreSQL to keep it in sync
	go func() {
		// Use background context to avoid cancellation
		bgCtx := context.Background()
		_, err := cs.db.Exec(bgCtx,
			`UPDATE credits SET balance = balance - $1, updated_at = NOW() 
			 WHERE user_id = $2 AND balance >= $1`,
			amount, userID,
		)
		if err != nil {
			cs.logger.Error().Err(err).
				Str("user_id", userID.String()).
				Int64("amount", amount).
				Msg("failed to sync credit deduction to PostgreSQL")
		} else {
			cs.logger.Debug().
				Str("user_id", userID.String()).
				Int64("amount", amount).
				Int64("new_balance", newBalance).
				Msg("credits synced to PostgreSQL")
		}
	}()

	cs.logger.Info().
		Str("user_id", userID.String()).
		Int64("amount", amount).
		Int64("new_balance", newBalance).
		Msg("credits reserved atomically")

	return true, newBalance, nil
}

// CheckCreditAvailability checks if user has sufficient credits without deducting.
// DEPRECATED: Use CheckAndReserve() instead to avoid race conditions.
func (cs *CreditService) CheckCreditAvailability(ctx context.Context, userID uuid.UUID, amount int64) (bool, int64, error) {
	balance, err := cs.cache.GetCredits(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to check credits: %w", err)
	}

	return balance >= amount, balance, nil
}

// ReserveCreditForSend reserves credits for an email send (creates a temporary hold).
func (cs *CreditService) ReserveCreditForSend(ctx context.Context, userID uuid.UUID, emailID uuid.UUID, amount int64) error {
	reservationKey := fmt.Sprintf("credit_reservation:%s:%s", userID.String(), emailID.String())

	// Create reservation record as JSON string
	reservationData := fmt.Sprintf(`{"user_id":"%s","email_id":"%s","amount":%d,"timestamp":%d,"status":"reserved"}`,
		userID.String(), emailID.String(), amount, time.Now().Unix())

	err := cs.cache.SetWithExpiry(ctx, reservationKey, reservationData, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to create credit reservation: %w", err)
	}

	cs.logger.Debug().
		Str("user_id", userID.String()).
		Str("email_id", emailID.String()).
		Int64("amount", amount).
		Msg("credit reservation created")

	return nil
}

// ConfirmCreditReservation confirms a credit reservation and deducts the credits.
func (cs *CreditService) ConfirmCreditReservation(ctx context.Context, userID uuid.UUID, emailID uuid.UUID) error {
	reservationKey := fmt.Sprintf("credit_reservation:%s:%s", userID.String(), emailID.String())

	// Get reservation details
	reservationData, err := cs.cache.Get(ctx, reservationKey)
	if err != nil {
		return fmt.Errorf("failed to get credit reservation: %w", err)
	}

	if reservationData == "" {
		return fmt.Errorf("credit reservation not found")
	}

	// For now, assume amount is 1 (can be enhanced later with JSON parsing)
	// Deduct the credits
	err = cs.DeductCredit(ctx, userID, 1)
	if err != nil {
		return fmt.Errorf("failed to confirm credit deduction: %w", err)
	}

	// Mark reservation as confirmed
	confirmedData := fmt.Sprintf(`{"status":"confirmed","confirmed_at":%d}`, time.Now().Unix())
	cs.cache.SetWithExpiry(ctx, reservationKey, confirmedData, 24*time.Hour)

	return nil
}

// CancelCreditReservation cancels a credit reservation without deducting credits.
func (cs *CreditService) CancelCreditReservation(ctx context.Context, userID uuid.UUID, emailID uuid.UUID, reason string) error {
	reservationKey := fmt.Sprintf("credit_reservation:%s:%s", userID.String(), emailID.String())

	// Mark reservation as cancelled
	cancelledData := fmt.Sprintf(`{"status":"cancelled","reason":"%s","cancelled_at":%d}`,
		reason, time.Now().Unix())
	cs.cache.SetWithExpiry(ctx, reservationKey, cancelledData, 24*time.Hour)

	cs.logger.Info().
		Str("user_id", userID.String()).
		Str("email_id", emailID.String()).
		Str("reason", reason).
		Msg("credit reservation cancelled")

	return nil
}
