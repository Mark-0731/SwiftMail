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
		panic(fmt.Sprintf("failed to generate secure token: %v", err))
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
func (s *service) checkRateLimit(ctx context.Context, key string, maxAttempts int, window time.Duration) error {
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to increment rate limit counter")
		return nil // Fail open for availability
	}

	if count == 1 {
		s.rdb.Expire(ctx, key, window)
	}

	if count > int64(maxAttempts) {
		return ErrRateLimitExceeded
	}

	return nil
}
