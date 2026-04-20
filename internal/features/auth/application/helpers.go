package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/features/auth"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/infrastructure"
)

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// This should never happen in practice, but if it does, we need to know
		panic(fmt.Sprintf("critical: failed to generate secure token: %v", err))
	}
	return hex.EncodeToString(b)
}

// hashToken creates a SHA-256 hash of a token
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// toUserResponse converts UserModel to UserResponse DTO
func toUserResponse(user *infrastructure.UserModel) auth.UserResponse {
	return auth.UserResponse{
		ID:            user.ID,
		Email:         user.Email,
		Name:          user.Name,
		Role:          user.Role,
		TOTPEnabled:   user.TOTPEnabled,
		EmailVerified: user.EmailVerified,
		Status:        user.Status,
		CreatedAt:     user.CreatedAt,
	}
}

// checkRateLimit checks if rate limit is exceeded using Redis
// Returns ErrRateLimitExceeded if limit is exceeded
// Fails open (returns nil) if Redis is unavailable for availability
func (s *service) checkRateLimit(ctx context.Context, key string, maxAttempts int, window time.Duration) error {
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		s.logger.Warn().
			Err(err).
			Str("key", key).
			Msg("failed to increment rate limit counter, failing open")
		return nil // Fail open for availability
	}

	// Set expiry on first increment
	if count == 1 {
		if err := s.rdb.Expire(ctx, key, window).Err(); err != nil {
			s.logger.Warn().
				Err(err).
				Str("key", key).
				Msg("failed to set rate limit expiry")
			// Continue - counter is still incremented
		}
	}

	if count > int64(maxAttempts) {
		s.logger.Warn().
			Str("key", key).
			Int64("count", count).
			Int("max", maxAttempts).
			Msg("rate limit exceeded")
		return ErrRateLimitExceeded
	}

	return nil
}
