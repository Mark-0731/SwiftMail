package webhook

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers webhook routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	w := router.Group("/webhooks")
	w.Get("/", handler.List)
	w.Post("/", handler.Create)
	w.Delete("/:id", handler.Delete)
	w.Put("/:id/toggle", handler.Toggle)
}
