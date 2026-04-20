package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// CreditService handles credit operations with proper rollback support.
type CreditService struct {
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewCreditService creates a new credit service.
func NewCreditService(rdb *redis.Client, logger zerolog.Logger) *CreditService {
	return &CreditService{
		rdb:    rdb,
		logger: logger,
	}
}

// DeductCredit deducts credits after successful email send.
func (cs *CreditService) DeductCredit(ctx context.Context, userID uuid.UUID, amount int64) error {
	creditKey := fmt.Sprintf("credits:%s", userID.String())

	// Atomic deduction using Lua script
	luaScript := `
		local balance = redis.call("GET", KEYS[1])
		if balance == false then
			return -2
		end
		if tonumber(balance) < tonumber(ARGV[1]) then
			return -1
		end
		return redis.call("DECRBY", KEYS[1], ARGV[1])
	`

	result, err := cs.rdb.Eval(ctx, luaScript, []string{creditKey}, amount).Int64()
	if err != nil {
		cs.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to deduct credits")
		return fmt.Errorf("failed to deduct credits: %w", err)
	}

	if result == -2 {
		cs.logger.Warn().Str("user_id", userID.String()).Msg("no credit record found for deduction")
		return fmt.Errorf("no credit record found")
	}

	if result == -1 {
		cs.logger.Warn().Str("user_id", userID.String()).Msg("insufficient credits for deduction")
		return fmt.Errorf("insufficient credits")
	}

	cs.logger.Info().
		Str("user_id", userID.String()).
		Int64("amount", amount).
		Int64("remaining", result).
		Msg("credits deducted successfully")

	return nil
}

// RefundCredit refunds credits if email send fails.
func (cs *CreditService) RefundCredit(ctx context.Context, userID uuid.UUID, amount int64, reason string) error {
	creditKey := fmt.Sprintf("credits:%s", userID.String())

	// Add credits back
	newBalance, err := cs.rdb.IncrBy(ctx, creditKey, amount).Result()
	if err != nil {
		cs.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to refund credits")
		return fmt.Errorf("failed to refund credits: %w", err)
	}

	// Log the refund for audit purposes
	refundKey := fmt.Sprintf("refund:%s:%d", userID.String(), time.Now().Unix())
	refundData := map[string]interface{}{
		"user_id":     userID.String(),
		"amount":      amount,
		"reason":      reason,
		"timestamp":   time.Now().Unix(),
		"new_balance": newBalance,
	}

	cs.rdb.HSet(ctx, refundKey, refundData)
	cs.rdb.Expire(ctx, refundKey, 30*24*time.Hour) // Keep refund records for 30 days

	cs.logger.Info().
		Str("user_id", userID.String()).
		Int64("amount", amount).
		Str("reason", reason).
		Int64("new_balance", newBalance).
		Msg("credits refunded successfully")

	return nil
}

// CheckCreditAvailability checks if user has sufficient credits without deducting.
func (cs *CreditService) CheckCreditAvailability(ctx context.Context, userID uuid.UUID, amount int64) (bool, int64, error) {
	creditKey := fmt.Sprintf("credits:%s", userID.String())

	balance, err := cs.rdb.Get(ctx, creditKey).Int64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return false, 0, fmt.Errorf("no credit record found")
		}
		return false, 0, fmt.Errorf("failed to check credits: %w", err)
	}

	return balance >= amount, balance, nil
}

// ReserveCreditForSend reserves credits for an email send (creates a temporary hold).
func (cs *CreditService) ReserveCreditForSend(ctx context.Context, userID uuid.UUID, emailID uuid.UUID, amount int64) error {
	reservationKey := fmt.Sprintf("credit_reservation:%s:%s", userID.String(), emailID.String())

	// Create reservation record
	reservationData := map[string]interface{}{
		"user_id":   userID.String(),
		"email_id":  emailID.String(),
		"amount":    amount,
		"timestamp": time.Now().Unix(),
		"status":    "reserved",
	}

	err := cs.rdb.HSet(ctx, reservationKey, reservationData).Err()
	if err != nil {
		return fmt.Errorf("failed to create credit reservation: %w", err)
	}

	// Set expiration (reservation expires in 1 hour if not confirmed)
	cs.rdb.Expire(ctx, reservationKey, 1*time.Hour)

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
	reservationData, err := cs.rdb.HGetAll(ctx, reservationKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get credit reservation: %w", err)
	}

	if len(reservationData) == 0 {
		return fmt.Errorf("credit reservation not found")
	}

	amount := reservationData["amount"]
	if amount == "" {
		return fmt.Errorf("invalid reservation data")
	}

	// Convert amount to int64
	var amountInt int64
	fmt.Sscanf(amount, "%d", &amountInt)

	// Deduct the credits
	err = cs.DeductCredit(ctx, userID, amountInt)
	if err != nil {
		return fmt.Errorf("failed to confirm credit deduction: %w", err)
	}

	// Mark reservation as confirmed
	cs.rdb.HSet(ctx, reservationKey, "status", "confirmed")
	cs.rdb.Expire(ctx, reservationKey, 24*time.Hour) // Keep for audit

	return nil
}

// CancelCreditReservation cancels a credit reservation without deducting credits.
func (cs *CreditService) CancelCreditReservation(ctx context.Context, userID uuid.UUID, emailID uuid.UUID, reason string) error {
	reservationKey := fmt.Sprintf("credit_reservation:%s:%s", userID.String(), emailID.String())

	// Mark reservation as cancelled
	cs.rdb.HSet(ctx, reservationKey, "status", "cancelled")
	cs.rdb.HSet(ctx, reservationKey, "cancel_reason", reason)
	cs.rdb.Expire(ctx, reservationKey, 24*time.Hour) // Keep for audit

	cs.logger.Info().
		Str("user_id", userID.String()).
		Str("email_id", emailID.String()).
		Str("reason", reason).
		Msg("credit reservation cancelled")

	return nil
}
