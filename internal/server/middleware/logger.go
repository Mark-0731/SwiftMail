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
// Captures request details, latency, and status for observability.
func Logger(log zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		requestID := uuid.New().String()

		// Store request ID in context and response header
		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)

		// Process request
		err := c.Next()

		// Calculate latency in milliseconds
		latencyMs := time.Since(start).Milliseconds()
		status := c.Response().StatusCode()

		// Structured logging with all relevant fields
		log.Info().
			Str("request_id", requestID).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Int64("latency_ms", latencyMs).
			Str("ip", c.IP()).
			Str("user_agent", c.Get("User-Agent")).
			Msg("request")

		return err
	}
}

// Metrics middleware records Prometheus metrics for all requests.
// Uses route patterns to prevent high-cardinality issues.
func Metrics(m *metrics.Metrics) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Capture method BEFORE c.Next() to avoid corruption
		method := string(c.Request().Header.Method())

		// Process request
		err := c.Next()

		// Capture path AFTER c.Next() to get route pattern
		path := getRoutePath(c)
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())

		// Record metrics
		m.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.APIRequestDuration.WithLabelValues(method, path).Observe(duration)

		return err
	}
}

// getRoutePath extracts the route pattern from the Fiber context.
// Returns the route pattern (e.g., "/users/:id") if available,
// otherwise returns the actual path to prevent nil pointer issues.
func getRoutePath(c *fiber.Ctx) string {
	// Try to get the route pattern first (preferred for low cardinality)
	if route := c.Route(); route != nil && route.Path != "" {
		return route.Path
	}

	// Fallback to actual path if route is not available
	// This can happen for 404s or before routing is complete
	path := c.Path()
	if path == "" {
		return "/"
	}

	return path
}

// MetricsWithInflight tracks in-flight requests using a Gauge.
// This is useful for monitoring concurrent request load.
func MetricsWithInflight(m *metrics.Metrics, inflightGauge *metrics.InflightGauge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		method := c.Method()
		path := getRoutePath(c)

		// Increment in-flight counter
		if inflightGauge != nil {
			inflightGauge.Inc(method, path)
			defer inflightGauge.Dec(method, path)
		}

		err := c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())

		m.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.APIRequestDuration.WithLabelValues(method, path).Observe(duration)

		return err
	}
}
