package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RateLimiter handles rate limiting for authentication endpoints.
type RateLimiter struct {
	rdb *redis.Client
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// LoginRateLimit middleware limits login attempts per IP and per email.
func (rl *RateLimiter) LoginRateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		// Rate limit by IP: 10 attempts per 15 minutes
		ipKey := fmt.Sprintf("rate:login:ip:%s", ip)
		ipCount, err := rl.rdb.Incr(c.Context(), ipKey).Result()
		if err == nil {
			if ipCount == 1 {
				rl.rdb.Expire(c.Context(), ipKey, 15*time.Minute)
			}
			if ipCount > 10 {
				return response.TooManyRequests(c, "Too many login attempts from this IP. Please try again later.")
			}
		}

		return c.Next()
	}
}

// SignupRateLimit middleware limits signup attempts per IP.
func (rl *RateLimiter) SignupRateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		// Rate limit by IP: 3 signups per hour
		ipKey := fmt.Sprintf("rate:signup:ip:%s", ip)
		ipCount, err := rl.rdb.Incr(c.Context(), ipKey).Result()
		if err == nil {
			if ipCount == 1 {
				rl.rdb.Expire(c.Context(), ipKey, 1*time.Hour)
			}
			if ipCount > 3 {
				return response.TooManyRequests(c, "Too many signup attempts. Please try again later.")
			}
		}

		return c.Next()
	}
}

// PasswordResetRateLimit middleware limits password reset requests.
func (rl *RateLimiter) PasswordResetRateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		// Rate limit by IP: 5 requests per hour
		ipKey := fmt.Sprintf("rate:password_reset:ip:%s", ip)
		ipCount, err := rl.rdb.Incr(c.Context(), ipKey).Result()
		if err == nil {
			if ipCount == 1 {
				rl.rdb.Expire(c.Context(), ipKey, 1*time.Hour)
			}
			if ipCount > 5 {
				return response.TooManyRequests(c, "Too many password reset requests. Please try again later.")
			}
		}

		return c.Next()
	}
}

// CheckAccountLock checks if an account is locked due to failed attempts.
func (rl *RateLimiter) CheckAccountLock(ctx context.Context, email string) (bool, time.Duration, error) {
	lockKey := fmt.Sprintf("account_lock:%s", email)
	ttl, err := rl.rdb.TTL(ctx, lockKey).Result()
	if err != nil {
		return false, 0, err
	}

	if ttl > 0 {
		return true, ttl, nil
	}

	return false, 0, nil
}

// LockAccount locks an account for a specified duration.
func (rl *RateLimiter) LockAccount(ctx context.Context, email string, duration time.Duration) error {
	lockKey := fmt.Sprintf("account_lock:%s", email)
	return rl.rdb.Set(ctx, lockKey, "1", duration).Err()
}

// UnlockAccount unlocks a locked account.
func (rl *RateLimiter) UnlockAccount(ctx context.Context, email string) error {
	lockKey := fmt.Sprintf("account_lock:%s", email)
	return rl.rdb.Del(ctx, lockKey).Err()
}

// IncrementFailedAttempts increments failed login attempts counter.
func (rl *RateLimiter) IncrementFailedAttempts(ctx context.Context, email string) (int64, error) {
	failKey := fmt.Sprintf("login_fail:%s", email)
	count, err := rl.rdb.Incr(ctx, failKey).Result()
	if err != nil {
		return 0, err
	}

	if count == 1 {
		rl.rdb.Expire(ctx, failKey, 1*time.Hour)
	}

	// Lock account after 5 failed attempts
	if count >= 5 {
		rl.LockAccount(ctx, email, 30*time.Minute)
	}

	return count, nil
}

// ClearFailedAttempts clears failed login attempts counter.
func (rl *RateLimiter) ClearFailedAttempts(ctx context.Context, email string) error {
	failKey := fmt.Sprintf("login_fail:%s", email)
	return rl.rdb.Del(ctx, failKey).Err()
}
