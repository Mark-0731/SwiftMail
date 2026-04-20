package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers warmup routes (admin only).
func RegisterRoutes(router fiber.Router, handler *Handler) {
	w := router.Group("/warmup")
	w.Get("/schedule", handler.GetSchedule)
	w.Get("/progress", handler.GetProgress)
}
