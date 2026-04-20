package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
)

// PasswordService handles password operations
type PasswordService interface {
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error
}

// RequestPasswordReset initiates password reset flow
func (s *service) RequestPasswordReset(ctx context.Context, email string) error {
	// Rate limit: max 3 requests per hour per email
	rateLimitKey := "password_reset_rate:" + email
	if err := s.checkRateLimit(ctx, rateLimitKey, 3, 1*time.Hour); err != nil {
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		// Don't reveal if email exists (security best practice)
		s.logger.Info().
			Str("email", email).
			Msg("password reset requested for non-existent email")
		return nil
	}

	// Generate secure reset token
	token := generateSecureToken(32)
	tokenHash := hashToken(token)

	// Store token with 1 hour expiry
	if err := s.repo.CreatePasswordResetToken(ctx, user.ID, tokenHash, time.Now().Add(1*time.Hour)); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", user.ID.String()).
			Msg("failed to create password reset token")
		return fmt.Errorf("failed to create reset token: %w", err)
	}

	// TODO: Send email with reset link via queue
	// resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	s.logger.Info().
		Str("user_id", user.ID.String()).
		Str("email", email).
		Msg("password reset token generated")

	return nil
}

// ResetPassword resets password using token
func (s *service) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := hashToken(token)

	// Get and validate token
	resetToken, err := s.repo.GetPasswordResetToken(ctx, tokenHash)
	if err != nil {
		s.logger.Warn().
			Err(err).
			Str("token_hash", tokenHash[:8]+"...").
			Msg("password reset token not found")
		return ErrInvalidToken
	}

	// Check if already used
	if resetToken.UsedAt != nil {
		s.logger.Warn().
			Str("user_id", resetToken.UserID.String()).
			Msg("password reset token already used")
		return ErrInvalidToken
	}

	// Check expiry
	if time.Now().After(resetToken.ExpiresAt) {
		s.logger.Warn().
			Str("user_id", resetToken.UserID.String()).
			Time("expired_at", resetToken.ExpiresAt).
			Msg("password reset token expired")

		// Clean up expired token
		if delErr := s.repo.DeletePasswordResetToken(ctx, tokenHash); delErr != nil {
			s.logger.Error().
				Err(delErr).
				Str("token_hash", tokenHash[:8]+"...").
				Msg("failed to delete expired password reset token")
		}

		return ErrInvalidToken
	}

	// Hash new password
	passwordHash, err := domain.HashPassword(newPassword)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", resetToken.UserID.String()).
			Msg("failed to hash new password")
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, resetToken.UserID, passwordHash); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", resetToken.UserID.String()).
			Msg("failed to update password")
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	if err := s.repo.MarkPasswordResetTokenUsed(ctx, tokenHash); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", resetToken.UserID.String()).
			Msg("failed to mark password reset token as used")
		// Don't fail - password is already updated
	}

	// Revoke all sessions for security
	if err := s.RevokeAllSessions(ctx, resetToken.UserID); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", resetToken.UserID.String()).
			Msg("failed to revoke sessions after password reset")
		// Don't fail - password is already updated
	}

	s.logger.Info().
		Str("user_id", resetToken.UserID.String()).
		Msg("password reset successful")

	return nil
}

// ChangePassword changes user password (requires current password)
func (s *service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to get user for password change")
		return ErrUserNotFound
	}

	// Verify current password
	if !domain.CheckPassword(currentPassword, user.PasswordHash) {
		s.logger.Warn().
			Str("user_id", userID.String()).
			Msg("incorrect current password provided")
		return ErrInvalidCredentials
	}

	// Hash new password
	passwordHash, err := domain.HashPassword(newPassword)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to hash new password")
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, userID, passwordHash); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to update password")
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Revoke all sessions for security
	if err := s.RevokeAllSessions(ctx, userID); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to revoke sessions after password change")
		// Don't fail - password is already updated
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("password changed successfully")

	return nil
}
