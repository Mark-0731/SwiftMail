package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers admin routes (requires owner role).
func RegisterRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/admin")
	a.Get("/health", handler.SystemHealth)
	a.Get("/users", handler.ListUsers)
	a.Post("/users/:id/suspend", handler.SuspendUser)
	a.Post("/users/:id/unsuspend", handler.UnsuspendUser)
}
