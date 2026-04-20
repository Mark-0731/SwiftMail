package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers template management routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	t := router.Group("/templates")
	t.Post("/", handler.Create)
	t.Get("/", handler.List)
	t.Get("/:id", handler.Get)
	t.Put("/:id", handler.Update)
	t.Delete("/:id", handler.Delete)
	t.Post("/:id/preview", handler.Preview)
	t.Post("/:id/duplicate", handler.Duplicate)
	t.Post("/:id/archive", handler.Archive)
	t.Post("/:id/restore", handler.Restore)
	t.Get("/:id/versions", handler.GetVersions)
	t.Post("/:id/versions/:version/rollback", handler.Rollback)
}
