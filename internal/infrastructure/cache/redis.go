package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// RedisCache implements the Cache interface using Redis.
type RedisCache struct {
	client *redis.Client
	logger zerolog.Logger
}

// NewRedisCache creates a new Redis cache implementation.
func NewRedisCache(client *redis.Client, logger zerolog.Logger) Cache {
	return &RedisCache{
		client: client,
		logger: logger,
	}
}

// IsSuppressed checks if an email is suppressed for a user.
func (c *RedisCache) IsSuppressed(ctx context.Context, userID uuid.UUID, email string) (bool, error) {
	emailHash := hashEmail(email)

	// Check user suppression list
	suppressed, err := c.client.SIsMember(ctx,
		fmt.Sprintf("suppress:%s", userID.String()),
		emailHash,
	).Result()
	if err != nil {
		return false, fmt.Errorf("user suppression check failed: %w", err)
	}
	if suppressed {
		return true, nil
	}

	// Check global suppression
	globalSuppressed, err := c.client.SIsMember(ctx, "suppress:global", emailHash).Result()
	if err != nil {
		c.logger.Warn().Err(err).Msg("global suppression check failed")
		return false, nil // Fail open for global check
	}

	return globalSuppressed, nil
}

// AddSuppression adds an email to the suppression list.
func (c *RedisCache) AddSuppression(ctx context.Context, userID uuid.UUID, email string) error {
	emailHash := hashEmail(email)
	return c.client.SAdd(ctx, fmt.Sprintf("suppress:%s", userID.String()), emailHash).Err()
}

// RemoveSuppression removes an email from the suppression list.
func (c *RedisCache) RemoveSuppression(ctx context.Context, userID uuid.UUID, email string) error {
	emailHash := hashEmail(email)
	return c.client.SRem(ctx, fmt.Sprintf("suppress:%s", userID.String()), emailHash).Err()
}

// GetCredits retrieves the credit balance for a user.
func (c *RedisCache) GetCredits(ctx context.Context, userID uuid.UUID) (int64, error) {
	creditKey := fmt.Sprintf("credits:%s", userID.String())
	return c.client.Get(ctx, creditKey).Int64()
}

// DeductCredits atomically deducts credits from a user's balance.
func (c *RedisCache) DeductCredits(ctx context.Context, userID uuid.UUID, amount int64) (int64, error) {
	creditKey := fmt.Sprintf("credits:%s", userID.String())

	// Atomic deduction using Lua script
	luaScript := `
		local balance = redis.call("GET", KEYS[1])
		if balance == false then
			return -2
		end
		if tonumber(balance) < tonumber(ARGV[1]) then
			return -1
		end
		return redis.call("DECRBY", KEYS[1], ARGV[1])
	`

	result, err := c.client.Eval(ctx, luaScript, []string{creditKey}, amount).Int64()
	if err != nil {
		return 0, fmt.Errorf("credit deduction failed: %w", err)
	}

	if result == -2 {
		return 0, fmt.Errorf("no credit record found")
	}

	if result == -1 {
		return 0, fmt.Errorf("insufficient credits")
	}

	return result, nil
}

// AddCredits adds credits to a user's balance.
func (c *RedisCache) AddCredits(ctx context.Context, userID uuid.UUID, amount int64) (int64, error) {
	creditKey := fmt.Sprintf("credits:%s", userID.String())
	return c.client.IncrBy(ctx, creditKey, amount).Result()
}

// GetAPIKey retrieves cached API key data.
func (c *RedisCache) GetAPIKey(ctx context.Context, keyHash string) (string, error) {
	apiKeyKey := fmt.Sprintf("apikey:%s", keyHash)
	return c.client.Get(ctx, apiKeyKey).Result()
}

// SetAPIKey caches API key data.
func (c *RedisCache) SetAPIKey(ctx context.Context, keyHash string, data string, ttl time.Duration) error {
	apiKeyKey := fmt.Sprintf("apikey:%s", keyHash)
	return c.client.Set(ctx, apiKeyKey, data, ttl).Err()
}

// DeleteAPIKey removes cached API key data.
func (c *RedisCache) DeleteAPIKey(ctx context.Context, keyHash string) error {
	apiKeyKey := fmt.Sprintf("apikey:%s", keyHash)
	return c.client.Del(ctx, apiKeyKey).Err()
}

// GetDomainID retrieves cached domain ID.
func (c *RedisCache) GetDomainID(ctx context.Context, userID uuid.UUID, domain string) (*uuid.UUID, error) {
	domainKey := fmt.Sprintf("domain:%s:%s", userID.String(), domain)
	domainIDStr, err := c.client.Get(ctx, domainKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found
		}
		return nil, err
	}

	domainID, err := uuid.Parse(domainIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid domain ID in cache: %w", err)
	}

	return &domainID, nil
}

// SetDomainID caches domain ID.
func (c *RedisCache) SetDomainID(ctx context.Context, userID uuid.UUID, domain string, domainID uuid.UUID, ttl time.Duration) error {
	domainKey := fmt.Sprintf("domain:%s:%s", userID.String(), domain)
	return c.client.Set(ctx, domainKey, domainID.String(), ttl).Err()
}

// IncrementCounter increments a counter with expiry.
func (c *RedisCache) IncrementCounter(ctx context.Context, key string, window time.Duration) (int64, error) {
	count, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Set expiry only on first increment
	if count == 1 {
		c.client.Expire(ctx, key, window)
	}

	return count, nil
}

// GetCounter retrieves a counter value.
func (c *RedisCache) GetCounter(ctx context.Context, key string) (int64, error) {
	return c.client.Get(ctx, key).Int64()
}

// SetWithExpiry sets a key with expiry.
func (c *RedisCache) SetWithExpiry(ctx context.Context, key string, value string, expiry time.Duration) error {
	return c.client.Set(ctx, key, value, expiry).Err()
}

// Get retrieves a value by key.
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Delete removes one or more keys.
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists.
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// SetNX sets a key only if it doesn't exist.
func (c *RedisCache) SetNX(ctx context.Context, key string, value string, expiry time.Duration) (bool, error) {
	return c.client.SetNX(ctx, key, value, expiry).Result()
}

// hashEmail creates a SHA-256 hash of an email address.
func hashEmail(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}
