package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/features/verification"
	"github.com/Mark-0731/SwiftMail/internal/platform/queue"
)

// Common errors
var (
	ErrInvalidToken      = errors.New("invalid or expired verification token")
	ErrAlreadyVerified   = errors.New("email already verified")
	ErrRateLimitExceeded = errors.New("rate limit exceeded, please try again later")
	ErrUserNotFound      = errors.New("user not found")
	ErrTokenExpired      = errors.New("verification token has expired")
	ErrTokenAlreadyUsed  = errors.New("verification token already used")
)

const (
	// Task types for queue
	TaskTypeVerificationEmail = "email:verification"

	// Rate limit keys
	rateLimitKeyPrefix = "verification:rate:"

	// Token settings
	tokenLength      = 32
	tokenExpiryHours = 24

	// Rate limits
	maxVerificationEmailsPerHour = 3
	rateLimitWindow              = 1 * time.Hour
)

// Service handles email verification business logic
type Service interface {
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, token string) error
	ResendVerificationEmail(ctx context.Context, userID uuid.UUID) error
}

type service struct {
	repo   verification.Repository
	queue  queue.Queue
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewService creates a new verification service
func NewService(
	repo verification.Repository,
	queue queue.Queue,
	rdb *redis.Client,
	logger zerolog.Logger,
) Service {
	return &service{
		repo:   repo,
		queue:  queue,
		rdb:    rdb,
		logger: logger,
	}
}

// SendVerificationEmail sends email verification link to user (async via queue)
func (s *service) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	// Get user details
	email, name, verified, err := s.repo.GetUserEmail(ctx, userID)
	if err != nil {
		s.logger.Error().Err(err).Str("user_id", userID.String()).Msg("failed to get user email")
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if already verified
	if verified {
		return ErrAlreadyVerified
	}

	// Rate limit: max 3 verification emails per hour per user
	if err := s.checkRateLimit(ctx, userID.String()); err != nil {
		s.logger.Warn().
			Err(err).
			Str("user_id", userID.String()).
			Msg("verification email rate limit exceeded")
		return err
	}

	// Generate secure verification token
	token, err := generateSecureToken(tokenLength)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to generate secure token")
		return fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := hashToken(token)
	expiresAt := time.Now().Add(tokenExpiryHours * time.Hour)

	// Store token in database (atomic operation)
	if err := s.repo.CreateVerificationToken(ctx, userID, tokenHash, expiresAt); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to create verification token")
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	// Enqueue email sending task (async)
	if err := s.enqueueVerificationEmail(ctx, userID, email, name, token); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to enqueue verification email")
		// Don't fail the request - token is created, email will be retried
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("email", email).
		Msg("verification email queued successfully")

	return nil
}

// VerifyEmail verifies user email using token
func (s *service) VerifyEmail(ctx context.Context, token string) error {
	if token == "" {
		return ErrInvalidToken
	}

	tokenHash := hashToken(token)

	// Get and validate token (atomic read)
	verifyToken, err := s.repo.GetVerificationToken(ctx, tokenHash)
	if err != nil {
		s.logger.Warn().
			Err(err).
			Str("token_hash", tokenHash[:8]+"...").
			Msg("verification token not found")
		return ErrInvalidToken
	}

	// Check if already used
	if verifyToken.UsedAt != nil {
		s.logger.Warn().
			Str("user_id", verifyToken.UserID.String()).
			Msg("verification token already used")
		return ErrTokenAlreadyUsed
	}

	// Check expiry
	if time.Now().After(verifyToken.ExpiresAt) {
		s.logger.Warn().
			Str("user_id", verifyToken.UserID.String()).
			Time("expired_at", verifyToken.ExpiresAt).
			Msg("verification token expired")

		// Clean up expired token
		if delErr := s.repo.DeleteToken(ctx, tokenHash); delErr != nil {
			s.logger.Error().Err(delErr).Msg("failed to delete expired token")
		}

		return ErrTokenExpired
	}

	// Mark email as verified (atomic operation)
	if err := s.repo.MarkEmailVerified(ctx, verifyToken.UserID); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", verifyToken.UserID.String()).
			Msg("failed to mark email as verified")
		return fmt.Errorf("failed to verify email: %w", err)
	}

	// Mark token as used (atomic operation)
	if err := s.repo.MarkTokenUsed(ctx, tokenHash); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", verifyToken.UserID.String()).
			Msg("failed to mark token as used")
		// Don't fail - email is already verified
	}

	s.logger.Info().
		Str("user_id", verifyToken.UserID.String()).
		Msg("email verified successfully")

	return nil
}

// ResendVerificationEmail resends verification email
func (s *service) ResendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	return s.SendVerificationEmail(ctx, userID)
}

// checkRateLimit checks if rate limit is exceeded using Redis
func (s *service) checkRateLimit(ctx context.Context, userID string) error {
	key := rateLimitKeyPrefix + userID

	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to increment rate limit counter")
		// Fail open for availability
		return nil
	}

	// Set expiry on first increment
	if count == 1 {
		if err := s.rdb.Expire(ctx, key, rateLimitWindow).Err(); err != nil {
			s.logger.Warn().Err(err).Msg("failed to set rate limit expiry")
		}
	}

	if count > int64(maxVerificationEmailsPerHour) {
		return ErrRateLimitExceeded
	}

	return nil
}

// enqueueVerificationEmail adds verification email task to queue
func (s *service) enqueueVerificationEmail(ctx context.Context, userID uuid.UUID, email, name, token string) error {
	payload := map[string]interface{}{
		"user_id": userID.String(),
		"email":   email,
		"name":    name,
		"token":   token,
		"type":    "verification",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := &queue.Task{
		Type:    TaskTypeVerificationEmail,
		Payload: payloadBytes,
	}

	opts := &queue.EnqueueOptions{
		Queue:    "email",
		MaxRetry: 3,
		TaskID:   fmt.Sprintf("verify:%s:%d", userID.String(), time.Now().Unix()),
		Timeout:  30 * time.Second,
	}

	return s.queue.EnqueueWithOptions(ctx, task, opts)
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashToken creates a SHA-256 hash of a token
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
