package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const apiKeyPrefix = "sm_live_"

// APIKeyManager handles API key generation, hashing, and Redis caching.
type APIKeyManager struct {
	rdb *redis.Client
}

// NewAPIKeyManager creates a new API key manager.
func NewAPIKeyManager(rdb *redis.Client) *APIKeyManager {
	return &APIKeyManager{rdb: rdb}
}

// GenerateAPIKey generates a new API key with a prefix and returns the raw key and its hash.
func (m *APIKeyManager) GenerateAPIKey() (rawKey string, keyHash string, prefix string, err error) {
	// Generate 32 bytes of random data
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	rawKey = apiKeyPrefix + hex.EncodeToString(b)
	prefix = rawKey[:12]

	// Hash with SHA-256 for fast lookup, then bcrypt for storage
	sha := sha256.Sum256([]byte(rawKey))
	keyHash = hex.EncodeToString(sha[:])

	return rawKey, keyHash, prefix, nil
}

// HashAPIKey hashes an API key with SHA-256 for Redis lookup.
func (m *APIKeyManager) HashAPIKey(rawKey string) string {
	sha := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sha[:])
}

// CachedAPIKeyData is the data stored in Redis for fast API key validation.
type CachedAPIKeyData struct {
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	Status      string    `json:"status"`
	RatePerSec  int       `json:"rate_per_sec"`
	RatePerDay  int       `json:"rate_per_day"`
}

// CacheAPIKey stores API key data in Redis for fast lookup.
func (m *APIKeyManager) CacheAPIKey(ctx context.Context, keyHash string, data *CachedAPIKeyData) error {
	key := fmt.Sprintf("api_key:%s", keyHash)
	return m.rdb.Set(ctx, key, fmt.Sprintf(
		`{"user_id":"%s","role":"%s","status":"%s","rate_per_sec":%d,"rate_per_day":%d}`,
		data.UserID, data.Role, data.Status, data.RatePerSec, data.RatePerDay,
	), 5*time.Minute).Err()
}

// GetCachedAPIKey retrieves API key data from Redis cache.
func (m *APIKeyManager) GetCachedAPIKey(ctx context.Context, keyHash string) (string, error) {
	key := fmt.Sprintf("api_key:%s", keyHash)
	return m.rdb.Get(ctx, key).Result()
}

// InvalidateAPIKeyCache removes an API key from Redis cache.
func (m *APIKeyManager) InvalidateAPIKeyCache(ctx context.Context, keyHash string) error {
	key := fmt.Sprintf("api_key:%s", keyHash)
	return m.rdb.Del(ctx, key).Err()
}

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a password against a bcrypt hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
