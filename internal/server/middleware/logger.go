package middleware

import (
	"strconv"
	"time"

	"github.com/Mark-0731/SwiftMail/pkg/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Logger middleware logs all requests with zerolog.
func Logger(log zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		requestID := uuid.New().String()
		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Dur("latency", duration).
			Str("ip", c.IP()).
			Str("user_agent", c.Get("User-Agent")).
			Msg("request")

		return err
	}
}

// Metrics middleware records Prometheus metrics for all requests.
func Metrics(m *metrics.Metrics) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Capture method and path BEFORE c.Next()
		method := c.Method()
		path := c.Path() // Use actual request path

		err := c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())

		m.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.APIRequestDuration.WithLabelValues(method, path).Observe(duration)

		return err
	}
}
