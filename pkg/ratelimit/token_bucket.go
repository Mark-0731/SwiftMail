package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenBucket implements a Redis-backed token bucket rate limiter.
type TokenBucket struct {
	rdb *redis.Client
}

// NewTokenBucket creates a new Redis-backed token bucket rate limiter.
func NewTokenBucket(rdb *redis.Client) *TokenBucket {
	return &TokenBucket{rdb: rdb}
}

// AllowPerSecond checks if a request is allowed within the per-second rate limit.
// Uses Redis INCR with a 2-second TTL sliding window.
func (tb *TokenBucket) AllowPerSecond(ctx context.Context, key string, limit int) (bool, error) {
	redisKey := fmt.Sprintf("rate:%s:sec", key)

	pipe := tb.rdb.Pipeline()
	incr := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, 2*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return incr.Val() <= int64(limit), nil
}

// AllowPerDay checks if a request is allowed within the daily rate limit.
func (tb *TokenBucket) AllowPerDay(ctx context.Context, key string, limit int) (bool, error) {
	redisKey := fmt.Sprintf("rate:%s:day", key)

	pipe := tb.rdb.Pipeline()
	incr := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, 24*time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return incr.Val() <= int64(limit), nil
}

// GetDailyUsage returns the current daily usage count for a key.
func (tb *TokenBucket) GetDailyUsage(ctx context.Context, key string) (int64, error) {
	redisKey := fmt.Sprintf("rate:%s:day", key)
	val, err := tb.rdb.Get(ctx, redisKey).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}
