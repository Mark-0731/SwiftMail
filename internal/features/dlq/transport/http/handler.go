package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/platform/queue"
	"github.com/Mark-0731/SwiftMail/internal/platform/resilience"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// DLQHandler handles DLQ management HTTP requests
type DLQHandler struct {
	dlq            *queue.DeadLetterQueue
	poisonQueue    *resilience.PoisonQueue
	circuitBreaker *resilience.CircuitBreakerManager
	adaptiveRetry  *resilience.AdaptiveRetryEngine
	backpressure   *resilience.BackpressureController
	queueClient    queue.Queue
	logger         zerolog.Logger
}

// NewDLQHandler creates a new DLQ handler
func NewDLQHandler(
	dlq *queue.DeadLetterQueue,
	poisonQueue *resilience.PoisonQueue,
	circuitBreaker *resilience.CircuitBreakerManager,
	adaptiveRetry *resilience.AdaptiveRetryEngine,
	backpressure *resilience.BackpressureController,
	queueClient queue.Queue,
	logger zerolog.Logger,
) *DLQHandler {
	return &DLQHandler{
		dlq:            dlq,
		poisonQueue:    poisonQueue,
		circuitBreaker: circuitBreaker,
		adaptiveRetry:  adaptiveRetry,
		backpressure:   backpressure,
		queueClient:    queueClient,
		logger:         logger,
	}
}

// ListDLQEntries lists DLQ entries with filtering
// GET /api/v1/admin/dlq
func (h *DLQHandler) ListDLQEntries(c *fiber.Ctx) error {
	filter := &queue.DLQFilter{
		TaskType:        c.Query("task_type"),
		ErrorCode:       c.Query("error_code"),
		RecipientDomain: c.Query("domain"),
		RetryStatus:     c.Query("status"),
		Limit:           50,
		Offset:          0,
	}

	if userID := c.Query("user_id"); userID != "" {
		uid, err := uuid.Parse(userID)
		if err == nil {
			filter.UserID = &uid
		}
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filter.Limit = l
		}
	}

	if offset := c.Query("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			filter.Offset = o
		}
	}

	if fromDate := c.Query("from_date"); fromDate != "" {
		if t, err := time.Parse(time.RFC3339, fromDate); err == nil {
			filter.FromDate = &t
		}
	}

	if toDate := c.Query("to_date"); toDate != "" {
		if t, err := time.Parse(time.RFC3339, toDate); err == nil {
			filter.ToDate = &t
		}
	}

	entries, total, err := h.dlq.List(c.Context(), filter)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to list DLQ entries")
		return response.InternalError(c, "Failed to list DLQ entries")
	}

	return response.OK(c, fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   filter.Limit,
		"offset":  filter.Offset,
	})
}

// GetDLQEntry retrieves a single DLQ entry
// GET /api/v1/admin/dlq/:id
func (h *DLQHandler) GetDLQEntry(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid DLQ entry ID")
	}

	entry, err := h.dlq.Get(c.Context(), id)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id.String()).Msg("failed to get DLQ entry")
		return response.NotFound(c, "DLQ entry not found")
	}

	return response.OK(c, entry)
}

// RetryDLQEntry retries a single DLQ entry
// POST /api/v1/admin/dlq/:id/retry
func (h *DLQHandler) RetryDLQEntry(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid DLQ entry ID")
	}

	err = h.dlq.Retry(c.Context(), id, h.queueClient)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id.String()).Msg("failed to retry DLQ entry")
		return response.InternalError(c, "Failed to retry DLQ entry")
	}

	return response.OK(c, fiber.Map{
		"message": "DLQ entry retried successfully",
		"id":      id,
	})
}

// RetryDLQBatch retries multiple DLQ entries
// POST /api/v1/admin/dlq/retry-batch
func (h *DLQHandler) RetryDLQBatch(c *fiber.Ctx) error {
	var req struct {
		IDs []string `json:"ids" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if len(req.IDs) == 0 {
		return response.BadRequest(c, "EMPTY_IDS", "IDs array is required")
	}

	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, idStr := range req.IDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return response.BadRequest(c, "INVALID_ID", "Invalid DLQ entry ID: "+idStr)
		}
		ids = append(ids, id)
	}

	successCount, errors := h.dlq.RetryBatch(c.Context(), ids, h.queueClient)

	return response.OK(c, fiber.Map{
		"success_count": successCount,
		"failed_count":  len(errors),
		"errors":        errors,
	})
}

// DeleteDLQEntry deletes a DLQ entry
// DELETE /api/v1/admin/dlq/:id
func (h *DLQHandler) DeleteDLQEntry(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid DLQ entry ID")
	}

	err = h.dlq.Delete(c.Context(), id)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id.String()).Msg("failed to delete DLQ entry")
		return response.InternalError(c, "Failed to delete DLQ entry")
	}

	return response.OK(c, fiber.Map{
		"message": "DLQ entry deleted successfully",
		"id":      id,
	})
}

// GetDLQStats returns DLQ statistics
// GET /api/v1/admin/dlq/stats
func (h *DLQHandler) GetDLQStats(c *fiber.Ctx) error {
	stats, err := h.dlq.GetStats(c.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to get DLQ stats")
		return response.InternalError(c, "Failed to get DLQ stats")
	}

	return response.OK(c, stats)
}

// ListPoisonQueue lists poison queue entries
// GET /api/v1/admin/poison-queue
func (h *DLQHandler) ListPoisonQueue(c *fiber.Ctx) error {
	filter := &resilience.PoisonQueueFilter{
		TaskType:        c.Query("task_type"),
		RecipientDomain: c.Query("domain"),
		Limit:           50,
		Offset:          0,
	}

	if userID := c.Query("user_id"); userID != "" {
		uid, err := uuid.Parse(userID)
		if err == nil {
			filter.UserID = &uid
		}
	}

	if reviewed := c.Query("reviewed"); reviewed != "" {
		r := reviewed == "true"
		filter.Reviewed = &r
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filter.Limit = l
		}
	}

	if offset := c.Query("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			filter.Offset = o
		}
	}

	entries, total, err := h.poisonQueue.List(c.Context(), filter)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to list poison queue entries")
		return response.InternalError(c, "Failed to list poison queue entries")
	}

	return response.OK(c, fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   filter.Limit,
		"offset":  filter.Offset,
	})
}

// MarkPoisonQueueReviewed marks a poison queue entry as reviewed
// POST /api/v1/admin/poison-queue/:id/review
func (h *DLQHandler) MarkPoisonQueueReviewed(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid poison queue entry ID")
	}

	var req struct {
		ReviewedBy uuid.UUID `json:"reviewed_by" validate:"required"`
		Notes      string    `json:"notes"`
		Action     string    `json:"action" validate:"required,oneof=retry discard manual_fix"`
	}

	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.ReviewedBy == uuid.Nil {
		return response.BadRequest(c, "MISSING_REVIEWER", "reviewed_by is required")
	}

	if req.Action != "retry" && req.Action != "discard" && req.Action != "manual_fix" {
		return response.BadRequest(c, "INVALID_ACTION", "action must be one of: retry, discard, manual_fix")
	}

	err = h.poisonQueue.MarkReviewed(c.Context(), id, req.ReviewedBy, req.Notes, req.Action)
	if err != nil {
		h.logger.Error().Err(err).Str("id", id.String()).Msg("failed to mark poison queue entry as reviewed")
		return response.InternalError(c, "Failed to mark as reviewed")
	}

	return response.OK(c, fiber.Map{
		"message": "Poison queue entry marked as reviewed",
		"id":      id,
	})
}

// GetPoisonQueueStats returns poison queue statistics
// GET /api/v1/admin/poison-queue/stats
func (h *DLQHandler) GetPoisonQueueStats(c *fiber.Ctx) error {
	stats, err := h.poisonQueue.GetStats(c.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to get poison queue stats")
		return response.InternalError(c, "Failed to get poison queue stats")
	}

	return response.OK(c, stats)
}

// GetCircuitBreakerStates returns all circuit breaker states
// GET /api/v1/admin/circuit-breakers
func (h *DLQHandler) GetCircuitBreakerStates(c *fiber.Ctx) error {
	states := h.circuitBreaker.GetAllStates()

	return response.OK(c, fiber.Map{
		"circuit_breakers": states,
	})
}

// ResetCircuitBreaker resets a specific circuit breaker
// POST /api/v1/admin/circuit-breakers/reset
func (h *DLQHandler) ResetCircuitBreaker(c *fiber.Ctx) error {
	var req struct {
		ResourceType string `json:"resource_type" validate:"required,oneof=provider domain"`
		ResourceID   string `json:"resource_id" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.ResourceType != "provider" && req.ResourceType != "domain" {
		return response.BadRequest(c, "INVALID_TYPE", "resource_type must be 'provider' or 'domain'")
	}

	if req.ResourceID == "" {
		return response.BadRequest(c, "MISSING_ID", "resource_id is required")
	}

	breaker := h.circuitBreaker.GetBreaker(req.ResourceType, req.ResourceID)
	breaker.Reset()

	return response.OK(c, fiber.Map{
		"message": "Circuit breaker reset successfully",
	})
}

// GetAdaptiveRetryStrategies returns all adaptive retry strategies
// GET /api/v1/admin/adaptive-retry
func (h *DLQHandler) GetAdaptiveRetryStrategies(c *fiber.Ctx) error {
	strategies := h.adaptiveRetry.GetAllStrategies()

	return response.OK(c, fiber.Map{
		"strategies": strategies,
	})
}

// GetBackpressureMetrics returns backpressure metrics
// GET /api/v1/admin/backpressure
func (h *DLQHandler) GetBackpressureMetrics(c *fiber.Ctx) error {
	metrics := h.backpressure.GetMetrics()

	return response.OK(c, metrics)
}

// ControlBackpressure controls backpressure system
// POST /api/v1/admin/backpressure/control
func (h *DLQHandler) ControlBackpressure(c *fiber.Ctx) error {
	var req struct {
		Action string `json:"action" validate:"required,oneof=pause resume enable disable reset"`
	}

	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	switch req.Action {
	case "pause":
		h.backpressure.Pause()
	case "resume":
		h.backpressure.Resume()
	case "enable":
		h.backpressure.Enable()
	case "disable":
		h.backpressure.Disable()
	case "reset":
		h.backpressure.Reset()
	default:
		return response.BadRequest(c, "INVALID_ACTION", "action must be one of: pause, resume, enable, disable, reset")
	}

	return response.OK(c, fiber.Map{
		"message": "Backpressure control action executed: " + req.Action,
	})
}
