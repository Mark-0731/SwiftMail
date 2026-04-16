package smtp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// CircuitBreaker implements a per-domain circuit breaker to protect against ISP rate limiting.
type CircuitBreaker struct {
	mu             sync.RWMutex
	rdb            *redis.Client
	logger         zerolog.Logger
	thresholdFail  int
	window         time.Duration
	cooldownTime   time.Duration
}

// NewCircuitBreaker creates a new circuit breaker manager.
func NewCircuitBreaker(rdb *redis.Client, logger zerolog.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		rdb:           rdb,
		logger:        logger,
		thresholdFail: 5,
		window:        1 * time.Minute,
		cooldownTime:  30 * time.Second,
	}
}

// AllowSend checks if sending to a domain is allowed.
func (cb *CircuitBreaker) AllowSend(ctx context.Context, domain string) (bool, error) {
	key := fmt.Sprintf("circuit:%s", domain)

	state, err := cb.rdb.HGet(ctx, key, "state").Result()
	if err == redis.Nil {
		return true, nil // No circuit breaker state = closed (allow)
	}
	if err != nil {
		return true, nil // Redis error = fail open
	}

	switch CircuitState(state) {
	case CircuitOpen:
		// Check if cooldown has passed
		openUntil, err := cb.rdb.HGet(ctx, key, "open_until").Result()
		if err == nil {
			t, _ := time.Parse(time.RFC3339, openUntil)
			if time.Now().After(t) {
				// Transition to half-open
				cb.rdb.HSet(ctx, key, "state", string(CircuitHalfOpen))
				return true, nil // Allow test request
			}
		}
		return false, nil // Still in cooldown

	case CircuitHalfOpen:
		return true, nil // Allow test request

	default:
		return true, nil // Closed = allow
	}
}

// RecordSuccess records a successful send to a domain.
func (cb *CircuitBreaker) RecordSuccess(ctx context.Context, domain string) {
	key := fmt.Sprintf("circuit:%s", domain)
	state, err := cb.rdb.HGet(ctx, key, "state").Result()
	if err != nil {
		return
	}

	if CircuitState(state) == CircuitHalfOpen {
		// Success in half-open → close circuit
		cb.rdb.Del(ctx, key)
		cb.logger.Info().Str("domain", domain).Msg("circuit breaker closed")
	}
}

// RecordFailure records a failed send to a domain.
func (cb *CircuitBreaker) RecordFailure(ctx context.Context, domain string) {
	key := fmt.Sprintf("circuit:%s", domain)

	state, _ := cb.rdb.HGet(ctx, key, "state").Result()

	if CircuitState(state) == CircuitHalfOpen {
		// Failure in half-open → re-open
		openUntil := time.Now().Add(cb.cooldownTime).Format(time.RFC3339)
		cb.rdb.HSet(ctx, key,
			"state", string(CircuitOpen),
			"open_until", openUntil,
		)
		cb.rdb.Expire(ctx, key, 5*time.Minute)
		cb.logger.Warn().Str("domain", domain).Msg("circuit breaker re-opened from half-open")
		return
	}

	// Increment failure count
	failures, _ := cb.rdb.HIncrBy(ctx, key, "failures", 1).Result()
	cb.rdb.Expire(ctx, key, cb.window)

	if failures >= int64(cb.thresholdFail) {
		// Trip the breaker
		openUntil := time.Now().Add(cb.cooldownTime).Format(time.RFC3339)
		cb.rdb.HSet(ctx, key,
			"state", string(CircuitOpen),
			"open_until", openUntil,
			"failures", 0,
		)
		cb.rdb.Expire(ctx, key, 5*time.Minute)
		cb.logger.Warn().Str("domain", domain).Int64("failures", failures).Msg("circuit breaker opened")
	}
}
