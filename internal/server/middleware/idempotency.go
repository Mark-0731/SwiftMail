package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Idempotency middleware prevents duplicate requests using Redis-backed idempotency keys.
func Idempotency(rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("Idempotency-Key")
		if key == "" {
			return c.Next()
		}

		// Validate UUID format
		if _, err := uuid.Parse(key); err != nil {
			return response.BadRequest(c, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a valid UUID")
		}

		ctx := c.Context()
		redisKey := "idempotency:" + key

		// Check if key already exists
		existing, err := rdb.Get(context.Background(), redisKey).Result()
		if err == nil && existing != "" {
			// Key exists — return cached response
			if existing == "processing" {
				return response.Conflict(c, "Request is currently being processed")
			}
			c.Set("Content-Type", "application/json")
			c.Set("X-Idempotent-Replayed", "true")
			return c.Status(fiber.StatusAccepted).SendString(existing)
		}

		// Mark as processing
		rdb.Set(context.Background(), redisKey, "processing", 24*time.Hour)

		// Process request
		err = c.Next()

		// Cache the response
		if c.Response().StatusCode() < 400 {
			body := string(c.Response().Body())
			rdb.Set(ctx, redisKey, body, 24*time.Hour)
		} else {
			// On error, remove the processing marker
			rdb.Del(ctx, redisKey)
		}

		return err
	}
}
