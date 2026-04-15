package analytics

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers analytics routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	a := router.Group("/analytics")
	a.Get("/overview", handler.Overview)
	a.Get("/timeseries", handler.TimeSeries)
	a.Get("/domains", handler.TopDomains)
}
