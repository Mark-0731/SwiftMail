package webhook

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds webhook HTTP handlers.
type Handler struct {
	repo       *Repository
	dispatcher *Dispatcher
}

// NewHandler creates webhook handlers.
func NewHandler(repo *Repository, dispatcher *Dispatcher) *Handler {
	return &Handler{repo: repo, dispatcher: dispatcher}
}

// RegisterRoutes registers webhook routes.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	w := router.Group("/webhooks")
	w.Get("/", h.List)
	w.Post("/", h.Create)
	w.Delete("/:id", h.Delete)
	w.Put("/:id/toggle", h.Toggle)
}

// List returns all webhooks for the authenticated user.
func (h *Handler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	webhooks, err := h.repo.GetByUserID(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to list webhooks")
	}

	// Strip secrets before returning
	for i := range webhooks {
		webhooks[i].Secret = ""
	}

	return response.OK(c, webhooks)
}

// Create creates a new webhook endpoint.
func (h *Handler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	var body CreateRequest
	if err := c.BodyParser(&body); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if body.URL == "" || len(body.Events) == 0 {
		return response.BadRequest(c, "MISSING_FIELDS", "URL and events are required")
	}

	wh := &Config{
		UserID: userID,
		URL:    body.URL,
		Events: body.Events,
		Active: true,
	}

	if err := h.repo.Create(c.Context(), wh); err != nil {
		return response.InternalError(c, "Failed to create webhook")
	}

	return response.Created(c, fiber.Map{
		"id":     wh.ID,
		"url":    wh.URL,
		"secret": wh.Secret, // Show only on creation
		"events": wh.Events,
	})
}

// Delete deletes a webhook.
func (h *Handler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid webhook ID")
	}

	if err := h.repo.Delete(c.Context(), id, userID); err != nil {
		return response.InternalError(c, "Failed to delete webhook")
	}

	return response.OK(c, fiber.Map{"message": "Webhook deleted"})
}

// Toggle enables or disables a webhook.
func (h *Handler) Toggle(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid webhook ID")
	}

	var body struct {
		Active bool `json:"active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if err := h.repo.ToggleActive(c.Context(), id, userID, body.Active); err != nil {
		return response.InternalError(c, "Failed to toggle webhook")
	}

	return response.OK(c, fiber.Map{"message": "Webhook updated", "active": body.Active})
}
