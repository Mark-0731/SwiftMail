package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Mark-0731/SwiftMail/internal/features/auth"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
)

// APIKeyService handles API key management
type APIKeyService interface {
	CreateAPIKey(ctx context.Context, userID uuid.UUID, req *auth.CreateAPIKeyRequest) (*auth.APIKeyResponse, error)
	ListAPIKeys(ctx context.Context, userID uuid.UUID) (*auth.APIKeyListResponse, error)
	DeleteAPIKey(ctx context.Context, keyID, userID uuid.UUID) error
}

// CreateAPIKey generates a new API key for user
func (s *service) CreateAPIKey(ctx context.Context, userID uuid.UUID, req *auth.CreateAPIKeyRequest) (*auth.APIKeyResponse, error) {
	// Generate API key
	rawKey, keyHash, prefix, err := s.apiKey.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Store in database
	key, err := s.repo.CreateAPIKey(ctx, userID, req.Name, keyHash, prefix, req.Permissions, req.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Get user for caching
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Cache API key in Redis for fast authentication
	cacheData := &domain.CachedAPIKeyData{
		UserID:      userID,
		Role:        user.Role,
		Status:      user.Status,
		RatePerSec:  100, // Default rate limits
		RatePerDay:  50000,
		Permissions: req.Permissions,
	}
	if err := s.apiKey.CacheAPIKey(ctx, keyHash, cacheData); err != nil {
		s.logger.Warn().Err(err).Msg("failed to cache API key")
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("key_prefix", prefix).
		Msg("API key created")

	return &auth.APIKeyResponse{
		ID:          key.ID,
		Name:        key.Name,
		KeyPrefix:   key.KeyPrefix,
		Key:         rawKey, // Only returned once
		Permissions: req.Permissions,
		LastUsedAt:  key.LastUsedAt,
		ExpiresAt:   key.ExpiresAt,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// ListAPIKeys retrieves all API keys for user
func (s *service) ListAPIKeys(ctx context.Context, userID uuid.UUID) (*auth.APIKeyListResponse, error) {
	keys, err := s.repo.GetAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	var resp []auth.APIKeyResponse
	for _, k := range keys {
		resp = append(resp, auth.APIKeyResponse{
			ID:          k.ID,
			Name:        k.Name,
			KeyPrefix:   k.KeyPrefix,
			Permissions: k.Permissions,
			LastUsedAt:  k.LastUsedAt,
			ExpiresAt:   k.ExpiresAt,
			CreatedAt:   k.CreatedAt,
		})
	}

	return &auth.APIKeyListResponse{Keys: resp}, nil
}

// DeleteAPIKey removes an API key
func (s *service) DeleteAPIKey(ctx context.Context, keyID, userID uuid.UUID) error {
	if err := s.repo.DeleteAPIKey(ctx, keyID, userID); err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("key_id", keyID.String()).
		Msg("API key deleted")

	return nil
}
