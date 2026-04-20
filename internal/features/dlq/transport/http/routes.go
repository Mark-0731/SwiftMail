package http

import (
	"github.com/gofiber/fiber/v2"
)

// RegisterRoutes registers DLQ management routes
func RegisterRoutes(router fiber.Router, handler *DLQHandler) {
	dlq := router.Group("/dlq")
	{
		// DLQ Management
		dlq.Get("", handler.ListDLQEntries)
		dlq.Get("/:id", handler.GetDLQEntry)
		dlq.Post("/:id/retry", handler.RetryDLQEntry)
		dlq.Post("/retry-batch", handler.RetryDLQBatch)
		dlq.Delete("/:id", handler.DeleteDLQEntry)
		dlq.Get("/stats", handler.GetDLQStats)
	}

	poison := router.Group("/poison-queue")
	{
		// Poison Queue Management
		poison.Get("", handler.ListPoisonQueue)
		poison.Post("/:id/review", handler.MarkPoisonQueueReviewed)
		poison.Get("/stats", handler.GetPoisonQueueStats)
	}

	circuitBreakers := router.Group("/circuit-breakers")
	{
		// Circuit Breaker Management
		circuitBreakers.Get("", handler.GetCircuitBreakerStates)
		circuitBreakers.Post("/reset", handler.ResetCircuitBreaker)
	}

	adaptiveRetry := router.Group("/adaptive-retry")
	{
		// Adaptive Retry Management
		adaptiveRetry.Get("", handler.GetAdaptiveRetryStrategies)
	}

	backpressure := router.Group("/backpressure")
	{
		// Backpressure Management
		backpressure.Get("", handler.GetBackpressureMetrics)
		backpressure.Post("/control", handler.ControlBackpressure)
	}
}
