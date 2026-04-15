package suppression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Service manages the suppression list with dual-write (PostgreSQL + Redis SET).
type Service struct {
	repo   Repository
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewService creates a suppression service.
func NewService(repo Repository, rdb *redis.Client, logger zerolog.Logger) *Service {
	return &Service{repo: repo, rdb: rdb, logger: logger}
}

// Add adds an email to the suppression list (dual-write).
func (s *Service) Add(ctx context.Context, userID uuid.UUID, email, suppressionType, reason string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	emailHash := hashEmail(email)

	// Write to PostgreSQL
	if err := s.repo.Add(ctx, userID, email, suppressionType, reason); err != nil {
		return err
	}

	// Write to Redis SET
	redisKey := fmt.Sprintf("suppress:%s", userID.String())
	s.rdb.SAdd(ctx, redisKey, emailHash)

	s.logger.Info().Str("email", email).Str("type", suppressionType).Msg("added to suppression list")
	return nil
}

// AddGlobal adds an email to the global suppression list.
func (s *Service) AddGlobal(ctx context.Context, email, suppressionType, reason string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	emailHash := hashEmail(email)

	if err := s.repo.AddGlobal(ctx, email, suppressionType, reason); err != nil {
		return err
	}

	s.rdb.SAdd(ctx, "suppress:global", emailHash)
	return nil
}

// IsSuppressed checks if an email is suppressed (Redis O(1) lookup).
func (s *Service) IsSuppressed(ctx context.Context, userID uuid.UUID, email string) (bool, error) {
	emailHash := hashEmail(strings.ToLower(strings.TrimSpace(email)))

	// Check user-specific suppression
	suppressed, err := s.rdb.SIsMember(ctx, fmt.Sprintf("suppress:%s", userID.String()), emailHash).Result()
	if err == nil && suppressed {
		return true, nil
	}

	// Check global suppression
	globalSuppressed, err := s.rdb.SIsMember(ctx, "suppress:global", emailHash).Result()
	if err == nil && globalSuppressed {
		return true, nil
	}

	return false, nil
}

// Remove removes an email from the suppression list.
func (s *Service) Remove(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	email, err := s.repo.Remove(ctx, id, userID)
	if err != nil {
		return err
	}

	// Remove from Redis
	emailHash := hashEmail(email)
	s.rdb.SRem(ctx, fmt.Sprintf("suppress:%s", userID.String()), emailHash)

	return nil
}

// List returns suppression list entries for a user.
func (s *Service) List(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Entry, int64, error) {
	return s.repo.List(ctx, userID, page, perPage)
}

func hashEmail(email string) string {
	h := sha256.Sum256([]byte(email))
	return hex.EncodeToString(h[:])
}
