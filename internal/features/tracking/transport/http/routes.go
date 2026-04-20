package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers public tracking routes (no auth required).
func RegisterRoutes(app *fiber.App, handler *Handler) {
	t := app.Group("/t")
	t.Get("/o/:id", handler.OpenPixel)
	t.Get("/c/:id", handler.ClickRedirect)
	t.Get("/u/:id", handler.Unsubscribe)
}
