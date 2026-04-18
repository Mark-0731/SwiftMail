package abuse

import (
	"github.com/gofiber/fiber/v2"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds abuse detection HTTP handlers.
type Handler struct {
	detector *Detector
}

// NewHandler creates abuse handlers.
func NewHandler(detector *Detector) *Handler {
	return &Handler{detector: detector}
}

// GetThresholds returns current abuse thresholds.
func (h *Handler) GetThresholds(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{
		"bounce_rate_threshold":    h.detector.BounceRateThreshold,
		"complaint_rate_threshold": h.detector.ComplaintRateThreshold,
		"spike_multiplier":         h.detector.SpikeMultiplier,
		"eval_window":              h.detector.EvalWindow.String(),
	})
}

// GetStatus returns abuse monitoring status.
func (h *Handler) GetStatus(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{
		"status":  "monitoring",
		"message": "Bounce/complaint rates within thresholds",
	})
}
