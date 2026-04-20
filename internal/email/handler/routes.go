package handler

import (
	"github.com/gofiber/fiber/v2"
)

// RegisterRoutes registers email sending and log routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	m := router.Group("/mail")
	m.Post("/send", handler.Send)

	l := router.Group("/logs")
	l.Get("/", handler.SearchLogs)
	l.Get("/:id", handler.GetLog)
}
