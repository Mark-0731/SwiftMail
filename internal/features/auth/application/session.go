package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SessionService handles session management operations
type SessionService interface {
	RevokeSession(ctx context.Context, refreshToken string) error
	RevokeAllSessions(ctx context.Context, userID uuid.UUID) error
}

// RevokeSession revokes a specific session by invalidating refresh token
func (s *service) RevokeSession(ctx context.Context, refreshToken string) error {
	// Validate refresh token
	claims, err := s.jwt.ValidateRefreshToken(refreshToken)
	if err != nil {
		s.logger.Warn().
			Err(err).
			Msg("invalid refresh token provided for revocation")
		return ErrInvalidToken
	}

	// Increment token version to invalidate all existing tokens
	if err := s.repo.IncrementTokenVersion(ctx, claims.UserID); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", claims.UserID.String()).
			Msg("failed to increment token version for session revocation")
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	s.logger.Info().
		Str("user_id", claims.UserID.String()).
		Msg("session revoked successfully")

	return nil
}

// RevokeAllSessions revokes all sessions for a user
func (s *service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	// Increment token version to invalidate all existing tokens
	if err := s.repo.IncrementTokenVersion(ctx, userID); err != nil {
		s.logger.Error().
			Err(err).
			Str("user_id", userID.String()).
			Msg("failed to increment token version for revoking all sessions")
		return fmt.Errorf("failed to revoke all sessions: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("all sessions revoked successfully")

	return nil
}
