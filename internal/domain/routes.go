package domain

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers domain management routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	d := router.Group("/domains")
	d.Post("/", handler.AddDomain)
	d.Get("/", handler.ListDomains)
	d.Get("/:id", handler.GetDomain)
	d.Post("/:id/verify", handler.VerifyDomain)
	d.Delete("/:id", handler.DeleteDomain)
}
