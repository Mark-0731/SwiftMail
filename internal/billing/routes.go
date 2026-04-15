package billing

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers billing routes.
func RegisterRoutes(router fiber.Router, handler *Handler) {
	b := router.Group("/billing")

	// Credit management
	b.Get("/credits", handler.GetCredits)
	b.Get("/usage", handler.GetUsage)
	b.Get("/transactions", handler.GetTransactions)

	// Plans
	b.Get("/plans", handler.GetPlans)

	// Subscriptions
	b.Get("/subscription", handler.GetSubscription)
	b.Post("/subscription/checkout", handler.CreateCheckoutSession)
	b.Post("/subscription/cancel", handler.CancelSubscription)

	// One-time purchases
	b.Post("/credits/purchase", handler.PurchaseCredits)
	b.Post("/payment-intent", handler.CreatePaymentIntent)

	// Stripe config
	b.Get("/config", handler.GetPublishableKey)
}

// RegisterWebhookRoutes registers public webhook routes (no auth required).
func RegisterWebhookRoutes(app *fiber.App, handler *Handler) {
	app.Post("/webhooks/stripe", handler.StripeWebhook)
}
