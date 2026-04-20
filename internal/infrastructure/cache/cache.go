package cache

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Cache defines the caching interface for the application.
// This abstraction allows us to swap Redis with other cache providers.
type Cache interface {
	// Suppression operations
	IsSuppressed(ctx context.Context, userID uuid.UUID, email string) (bool, error)
	AddSuppression(ctx context.Context, userID uuid.UUID, email string) error
	RemoveSuppression(ctx context.Context, userID uuid.UUID, email string) error

	// Credit operations
	GetCredits(ctx context.Context, userID uuid.UUID) (int64, error)
	DeductCredits(ctx context.Context, userID uuid.UUID, amount int64) (int64, error)
	AddCredits(ctx context.Context, userID uuid.UUID, amount int64) (int64, error)

	// API Key operations
	GetAPIKey(ctx context.Context, keyHash string) (string, error)
	SetAPIKey(ctx context.Context, keyHash string, data string, ttl time.Duration) error
	DeleteAPIKey(ctx context.Context, keyHash string) error

	// Domain operations
	GetDomainID(ctx context.Context, userID uuid.UUID, domain string) (*uuid.UUID, error)
	SetDomainID(ctx context.Context, userID uuid.UUID, domain string, domainID uuid.UUID, ttl time.Duration) error

	// Rate limiting operations
	IncrementCounter(ctx context.Context, key string, window time.Duration) (int64, error)
	GetCounter(ctx context.Context, key string) (int64, error)
	SetWithExpiry(ctx context.Context, key string, value string, expiry time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, keys ...string) error

	// Generic operations
	Exists(ctx context.Context, key string) (bool, error)
	SetNX(ctx context.Context, key string, value string, expiry time.Duration) (bool, error)
}

// CachedAPIKeyData represents cached API key information.
type CachedAPIKeyData struct {
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	RatePerSec  int       `json:"rate_per_sec"`
	RatePerDay  int       `json:"rate_per_day"`
	Permissions []string  `json:"permissions"`
}
