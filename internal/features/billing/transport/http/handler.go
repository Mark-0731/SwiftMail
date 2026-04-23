package http

import (
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

// CreatePaymentIntent creates a payment intent for credit top-up.
func (h *Handler) CreatePaymentIntent(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var req struct {
		AmountUSD int64 `json:"amount_usd"` // in USD (e.g., 10 = $10)
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.AmountUSD < billing.MinimumTopUpUSD {
		return response.ValidationError(c, "minimum top-up amount is $10")
	}

	if req.AmountUSD > 10000 {
		return response.ValidationError(c, "maximum top-up amount is $10,000")
	}

	pi, err := h.service.CreatePaymentIntent(c.Context(), userID, req.AmountUSD)
	if err != nil {
		return response.InternalError(c, err.Error())
	}

	credits := billing.CalculateCredits(req.AmountUSD)

	return response.OK(c, fiber.Map{
		"client_secret":     pi.ClientSecret,
		"payment_intent_id": pi.ID,
		"amount_usd":        req.AmountUSD,
		"credits":           credits,
	})
}

// ConfirmPayment confirms a payment and adds credits immediately.
func (h *Handler) ConfirmPayment(c *fiber.Ctx) error {
	var req struct {
		PaymentIntentID string `json:"payment_intent_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.PaymentIntentID == "" {
		return response.ValidationError(c, "payment_intent_id is required")
	}

	err := h.service.ConfirmPaymentAndAddCredits(c.Context(), req.PaymentIntentID)
	if err != nil {
		return response.InternalError(c, err.Error())
	}

	return response.OK(c, fiber.Map{"message": "Payment confirmed, credits added"})
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
