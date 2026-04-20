package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestID adds a unique request ID to each request.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check if request ID already exists in header
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in context
		c.Locals("request_id", requestID)

		// Add to response header
		c.Set("X-Request-ID", requestID)

		return c.Next()
	}
}
