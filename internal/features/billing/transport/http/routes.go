package http

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers billing routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	b := router.Group("/billing")

	// Credit management
	b.Get("/credits", handler.GetCredits)
	b.Get("/usage", handler.GetUsage)
	b.Get("/transactions", handler.GetTransactions)

	// Top-up (one-time payment)
	b.Post("/payment-intent", handler.CreatePaymentIntent)
	b.Post("/confirm-payment", handler.ConfirmPayment)
}
