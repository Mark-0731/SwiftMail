package http

import (
	"io"
	"strconv"

	"github.com/Mark-0731/SwiftMail/internal/features/billing"
	"github.com/Mark-0731/SwiftMail/internal/features/billing/application"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
)

// Handler holds billing HTTP handlers.
type Handler struct {
	service *application.Service
}

// NewHandler creates billing handlers.
func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

// GetCredits returns current credit balance.
func (h *Handler) GetCredits(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	credits, err := h.service.GetCredits(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to get credits")
	}
	return response.OK(c, credits)
}

// GetUsage returns monthly usage stats.
func (h *Handler) GetUsage(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	usage, err := h.service.GetUsage(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to get usage")
	}
	return response.OK(c, usage)
}

// GetPlans returns available billing plans.
func (h *Handler) GetPlans(c *fiber.Ctx) error {
	return response.OK(c, billing.AvailablePlans)
}

// GetSubscription returns the user's current subscription.
func (h *Handler) GetSubscription(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	sub, err := h.service.GetSubscription(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to get subscription")
	}
	if sub == nil {
		return response.OK(c, fiber.Map{"subscription": nil, "plan": "free"})
	}
	return response.OK(c, sub)
}

// CreateCheckoutSession creates a Stripe checkout session for subscription.
func (h *Handler) CreateCheckoutSession(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		PlanID string `json:"plan_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.PlanID == "" || req.PlanID == "free" {
		return response.ValidationError(c, "plan_id is required and cannot be 'free'")
	}

	session, err := h.service.CreateCheckoutSession(c.Context(), userID, req.PlanID)
	if err != nil {
		return response.InternalError(c, "Failed to create checkout session")
	}

	return response.OK(c, fiber.Map{
		"session_id": session.ID,
		"url":        session.URL,
	})
}

// CreatePaymentIntent creates a payment intent for credit purchase.
func (h *Handler) CreatePaymentIntent(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		Amount int64 `json:"amount"` // in USD cents
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Amount < 100 || req.Amount > 1000000 {
		return response.ValidationError(c, "amount must be between $1 and $10,000")
	}

	pi, err := h.service.CreatePaymentIntent(c.Context(), userID, req.Amount)
	if err != nil {
		return response.InternalError(c, "Failed to create payment intent")
	}

	return response.OK(c, fiber.Map{
		"client_secret":     pi.ClientSecret,
		"payment_intent_id": pi.ID,
	})
}

// CancelSubscription cancels the user's subscription.
func (h *Handler) CancelSubscription(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	err := h.service.CancelSubscription(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to cancel subscription")
	}

	return response.OK(c, fiber.Map{"message": "Subscription will be cancelled at the end of the billing period"})
}

// StripeWebhook handles Stripe webhook events.
func (h *Handler) StripeWebhook(c *fiber.Ctx) error {
	payload, err := io.ReadAll(c.Request().BodyStream())
	if err != nil {
		return response.BadRequest(c, "INVALID_PAYLOAD", "Failed to read request body")
	}

	signature := c.Get("Stripe-Signature")
	if signature == "" {
		return response.BadRequest(c, "MISSING_SIGNATURE", "Missing Stripe-Signature header")
	}

	event, err := h.service.ConstructWebhookEvent(payload, signature)
	if err != nil {
		return response.BadRequest(c, "INVALID_SIGNATURE", "Invalid webhook signature")
	}

	// Process webhook asynchronously
	go func() {
		if err := h.service.HandleWebhook(c.Context(), event); err != nil {
			// Log error but don't expose details
		}
	}()

	return response.OK(c, fiber.Map{"received": true})
}

// GetPublishableKey returns the Stripe publishable key for frontend.
func (h *Handler) GetPublishableKey(c *fiber.Ctx) error {
	return response.OK(c, fiber.Map{
		"publishable_key": h.service.GetStripePublishableKey(),
	})
}

// PurchaseCredits handles credit purchase (one-time payment).
func (h *Handler) PurchaseCredits(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		Credits int64 `json:"credits"` // Number of credits to purchase
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Credits < 100 || req.Credits > 1000000 {
		return response.ValidationError(c, "credits must be between 100 and 1,000,000")
	}

	// 1 credit = 1 cent
	amount := req.Credits

	pi, err := h.service.CreatePaymentIntent(c.Context(), userID, amount)
	if err != nil {
		return response.InternalError(c, "Failed to create payment intent")
	}

	return response.OK(c, fiber.Map{
		"client_secret":     pi.ClientSecret,
		"payment_intent_id": pi.ID,
		"amount":            amount,
		"credits":           req.Credits,
	})
}

// GetTransactions returns credit transaction history.
func (h *Handler) GetTransactions(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	offset := (page - 1) * perPage

	transactions, err := h.service.GetTransactions(c.Context(), userID, perPage, offset)
	if err != nil {
		return response.InternalError(c, "Failed to get transactions")
	}

	return response.OK(c, fiber.Map{
		"transactions": transactions,
		"page":         page,
		"per_page":     perPage,
	})
}
