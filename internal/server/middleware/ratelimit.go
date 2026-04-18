package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/Mark-0731/SwiftMail/pkg/ratelimit"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// RateLimit middleware enforces per-user rate limits.
func RateLimit(limiter *ratelimit.TokenBucket, perSec, perDay int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := GetUserID(c)
		if userID == uuid.Nil {
			return c.Next() // No rate limit for unauthenticated requests
		}

		key := userID.String()

		// Check per-second limit
		allowed, err := limiter.AllowPerSecond(c.Context(), key, perSec)
		if err != nil {
			// Redis down — allow request but log
			return c.Next()
		}
		if !allowed {
			return response.TooManyRequests(c, "Rate limit exceeded (per-second)")
		}

		// Check per-day limit
		allowed, err = limiter.AllowPerDay(c.Context(), key, perDay)
		if err != nil {
			return c.Next()
		}
		if !allowed {
			return response.TooManyRequests(c, "Daily rate limit exceeded")
		}

		return c.Next()
	}
}
