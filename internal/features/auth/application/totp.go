package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Mark-0731/SwiftMail/internal/features/auth"
)

// TOTPService handles two-factor authentication
type TOTPService interface {
	SetupTOTP(ctx context.Context, userID uuid.UUID) (*auth.TOTPSetupResponse, error)
	VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error
	DisableTOTP(ctx context.Context, userID uuid.UUID) error
}

// SetupTOTP generates TOTP secret for user
func (s *service) SetupTOTP(ctx context.Context, userID uuid.UUID) (*auth.TOTPSetupResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Generate TOTP secret and QR code URL
	secret, url, err := s.totp.GenerateSecret(user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP: %w", err)
	}

	// Store secret in database (not enabled yet)
	if err := s.repo.UpdateTOTPSecret(ctx, userID, secret); err != nil {
		return nil, fmt.Errorf("failed to store TOTP secret: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("TOTP setup initiated")

	return &auth.TOTPSetupResponse{
		Secret: secret,
		URL:    url,
	}, nil
}

// VerifyTOTP verifies TOTP code and enables 2FA
func (s *service) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if user.TOTPSecret == nil {
		return fmt.Errorf("TOTP not set up")
	}

	// Validate TOTP code
	if !s.totp.ValidateWithSkew(*user.TOTPSecret, code) {
		return ErrTOTPInvalid
	}

	// Enable TOTP for user
	if err := s.repo.EnableTOTP(ctx, userID); err != nil {
		return fmt.Errorf("failed to enable TOTP: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("TOTP enabled")

	return nil
}

// DisableTOTP disables two-factor authentication
func (s *service) DisableTOTP(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.DisableTOTP(ctx, userID); err != nil {
		return fmt.Errorf("failed to disable TOTP: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("TOTP disabled")

	return nil
}
