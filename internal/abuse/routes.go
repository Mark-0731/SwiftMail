package abuse

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers abuse detection routes (admin only).
func RegisterRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/abuse")
	a.Get("/thresholds", handler.GetThresholds)
	a.Get("/status", handler.GetStatus)
}
