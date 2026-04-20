package http

import (
	warmupapp "github.com/Mark-0731/SwiftMail/internal/features/warmup/application"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
)

// Handler holds warmup HTTP handlers.
type Handler struct {
	scheduler *warmupapp.Scheduler
}

// NewHandler creates warmup handlers.
func NewHandler(scheduler *warmupapp.Scheduler) *Handler {
	return &Handler{scheduler: scheduler}
}

// GetSchedule returns the current IP warmup schedule.
func (h *Handler) GetSchedule(c *fiber.Ctx) error {
	schedule := make([]fiber.Map, len(warmupapp.Schedule))
	for i, s := range warmupapp.Schedule {
		schedule[i] = fiber.Map{"day": s.Day, "limit": s.Limit}
	}
	return response.OK(c, fiber.Map{
		"total_days":       len(warmupapp.Schedule),
		"schedule":         schedule,
		"isp_distribution": warmupapp.ISPDistribution,
	})
}

// GetProgress returns warmup progress for an IP.
func (h *Handler) GetProgress(c *fiber.Ctx) error {
	ip := c.Query("ip", "")
	if ip == "" {
		return response.BadRequest(c, "MISSING_IP", "IP address required")
	}

	// Query warmup day from DB for this IP
	day, limit, err := h.scheduler.GetIPProgress(c.Context(), ip)
	if err != nil {
		return response.NotFound(c, "IP not found in warmup")
	}

	return response.OK(c, fiber.Map{
		"ip":          ip,
		"current_day": day,
		"daily_limit": limit,
		"total_days":  len(warmupapp.Schedule),
		"percentage":  float64(day) / float64(len(warmupapp.Schedule)) * 100,
	})
}
