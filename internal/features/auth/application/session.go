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
		return ErrInvalidToken
	}

	// Increment token version to invalidate all existing tokens
	if err := s.repo.IncrementTokenVersion(ctx, claims.UserID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	s.logger.Info().
		Str("user_id", claims.UserID.String()).
		Msg("session revoked")

	return nil
}

// RevokeAllSessions revokes all sessions for a user
func (s *service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	// Increment token version to invalidate all existing tokens
	if err := s.repo.IncrementTokenVersion(ctx, userID); err != nil {
		return fmt.Errorf("failed to revoke all sessions: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Msg("all sessions revoked")

	return nil
}
