package warmup

import (
	"github.com/gofiber/fiber/v2"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds warmup HTTP handlers.
type Handler struct {
	scheduler *Scheduler
}

// NewHandler creates warmup handlers.
func NewHandler(scheduler *Scheduler) *Handler {
	return &Handler{scheduler: scheduler}
}

// GetSchedule returns the current IP warmup schedule.
func (h *Handler) GetSchedule(c *fiber.Ctx) error {
	schedule := make([]fiber.Map, len(Schedule))
	for i, s := range Schedule {
		schedule[i] = fiber.Map{"day": s.Day, "limit": s.Limit}
	}
	return response.OK(c, fiber.Map{
		"total_days":       len(Schedule),
		"schedule":         schedule,
		"isp_distribution": ISPDistribution,
	})
}

// GetProgress returns warmup progress for an IP.
func (h *Handler) GetProgress(c *fiber.Ctx) error {
	ip := c.Query("ip", "")
	if ip == "" {
		return response.BadRequest(c, "MISSING_IP", "IP address required")
	}

	// Query warmup day from DB for this IP
	var day, limit int
	err := h.scheduler.db.QueryRow(c.Context(),
		`SELECT warmup_day, daily_limit FROM ip_addresses WHERE ip = $1`, ip,
	).Scan(&day, &limit)
	if err != nil {
		return response.NotFound(c, "IP not found in warmup")
	}

	return response.OK(c, fiber.Map{
		"ip":          ip,
		"current_day": day,
		"daily_limit": limit,
		"total_days":  len(Schedule),
		"percentage":  float64(day) / float64(len(Schedule)) * 100,
	})
}
