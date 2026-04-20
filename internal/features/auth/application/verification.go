package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// VerificationService handles email verification operations
type VerificationService interface {
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, token string) error
	ResendVerificationEmail(ctx context.Context, userID uuid.UUID) error
}

// SendVerificationEmail sends email verification link to user
func (s *service) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Check if already verified
	if user.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Rate limit: max 3 verification emails per hour
	if err := s.checkRateLimit(ctx, "email_verify_rate:"+userID.String(), 3, 1*time.Hour); err != nil {
		return err
	}

	// Generate secure verification token
	token := generateSecureToken(32)
	tokenHash := hashToken(token)

	// Store token with 24 hour expiry
	if err := s.repo.CreateEmailVerificationToken(ctx, userID, tokenHash, time.Now().Add(24*time.Hour)); err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	// TODO: Send email with verification link
	// verifyLink := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("verification email sent")

	return nil
}

// VerifyEmail verifies user email using token
func (s *service) VerifyEmail(ctx context.Context, token string) error {
	tokenHash := hashToken(token)

	// Get and validate token
	verifyToken, err := s.repo.GetEmailVerificationToken(ctx, tokenHash)
	if err != nil {
		return ErrInvalidToken
	}

	// Check expiry
	if time.Now().After(verifyToken.ExpiresAt) {
		s.repo.DeleteEmailVerificationToken(ctx, tokenHash)
		return ErrInvalidToken
	}

	// Check if already used
	if verifyToken.UsedAt != nil {
		return ErrInvalidToken
	}

	// Mark email as verified
	if err := s.repo.MarkEmailVerified(ctx, verifyToken.UserID); err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	// Mark token as used
	s.repo.MarkEmailVerificationTokenUsed(ctx, tokenHash)

	s.logger.Info().
		Str("user_id", verifyToken.UserID.String()).
		Msg("email verified successfully")

	return nil
}

// ResendVerificationEmail resends verification email
func (s *service) ResendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	return s.SendVerificationEmail(ctx, userID)
}
