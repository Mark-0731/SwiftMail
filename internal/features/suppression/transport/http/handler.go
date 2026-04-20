package http

import (
	suppression "github.com/Mark-0731/SwiftMail/internal/features/suppression"
	"github.com/Mark-0731/SwiftMail/internal/features/suppression/application"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Handler holds suppression HTTP handlers.
type Handler struct {
	service *application.Service
}

// NewHandler creates suppression handlers.
func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

// List returns all suppression entries for the authenticated user.
func (h *Handler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 50)

	entries, total, err := h.service.List(c.Context(), userID, page, perPage)
	if err != nil {
		return response.InternalError(c, "Failed to list suppression entries")
	}

	return response.Paginated(c, entries, response.Meta{Page: page, PerPage: perPage, Total: total})
}

// Add adds an email to the suppression list.
func (h *Handler) Add(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	var body suppression.AddRequest
	if err := c.BodyParser(&body); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if body.Email == "" || body.Type == "" {
		return response.BadRequest(c, "MISSING_FIELDS", "Email and type are required")
	}

	if err := h.service.Add(c.Context(), userID, body.Email, body.Type, body.Reason); err != nil {
		return response.InternalError(c, "Failed to add suppression entry")
	}

	return response.Created(c, map[string]string{"message": "Email suppressed"})
}

// Remove removes an email from the suppression list.
func (h *Handler) Remove(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid ID")
	}

	if err := h.service.Remove(c.Context(), id, userID); err != nil {
		return response.InternalError(c, "Failed to remove suppression entry")
	}

	return response.OK(c, map[string]string{"message": "Suppression removed"})
}
