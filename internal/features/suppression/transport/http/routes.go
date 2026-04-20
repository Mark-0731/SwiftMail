package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers suppression routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	s := router.Group("/suppression")
	s.Get("/", handler.List)
	s.Post("/", handler.Add)
	s.Delete("/:id", handler.Remove)
}
