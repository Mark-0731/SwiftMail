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
	if err := s.checkRateLimit(ctx, "password_reset_rate:"+email, 3, 1*time.Hour); err != nil {
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		// Don't reveal if email exists (security)
		s.logger.Info().Str("email", email).Msg("password reset requested for non-existent email")
		return nil
	}

	// Generate secure reset token
	token := generateSecureToken(32)
	tokenHash := hashToken(token)

	// Store token with 1 hour expiry
	if err := s.repo.CreatePasswordResetToken(ctx, user.ID, tokenHash, time.Now().Add(1*time.Hour)); err != nil {
		return fmt.Errorf("failed to create reset token: %w", err)
	}

	// TODO: Send email with reset link
	// resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)

	s.logger.Info().
		Str("user_id", user.ID.String()).
		Msg("password reset token generated")

	return nil
}

// ResetPassword resets password using token
func (s *service) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := hashToken(token)

	// Get and validate token
	resetToken, err := s.repo.GetPasswordResetToken(ctx, tokenHash)
	if err != nil {
		return ErrInvalidToken
	}

	// Check expiry
	if time.Now().After(resetToken.ExpiresAt) {
		s.repo.DeletePasswordResetToken(ctx, tokenHash)
		return ErrInvalidToken
	}

	// Check if already used
	if resetToken.UsedAt != nil {
		return ErrInvalidToken
	}

	// Hash new password
	passwordHash, err := domain.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, resetToken.UserID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	s.repo.MarkPasswordResetTokenUsed(ctx, tokenHash)

	// Revoke all sessions for security
	s.RevokeAllSessions(ctx, resetToken.UserID)

	s.logger.Info().
		Str("user_id", resetToken.UserID.String()).
		Msg("password reset successful")

	return nil
}

// ChangePassword changes user password (requires current password)
func (s *service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify current password
	if !domain.CheckPassword(currentPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	// Hash new password
	passwordHash, err := domain.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Revoke all sessions for security
	s.RevokeAllSessions(ctx, userID)

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("password changed successfully")

	return nil
}
