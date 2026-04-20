package http

import (
	"github.com/Mark-0731/SwiftMail/internal/features/admin/application"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Handler handles admin HTTP requests
type Handler struct {
	service application.Service
}

// NewHandler creates a new admin handler
func NewHandler(service application.Service) *Handler {
	return &Handler{service: service}
}

// ListUsers lists all users (admin only)
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 50)

	users, total, err := h.service.ListUsers(c.Context(), page, perPage)
	if err != nil {
		return response.InternalError(c, "Failed to list users")
	}

	return response.Paginated(c, users, response.Meta{
		Page:    page,
		PerPage: perPage,
		Total:   total,
	})
}

// SuspendUser suspends a user account
func (h *Handler) SuspendUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid user ID")
	}

	// Get reason from request body (optional)
	var req struct {
		Reason string `json:"reason"`
	}
	c.BodyParser(&req)

	if err := h.service.SuspendUser(c.Context(), id, req.Reason); err != nil {
		return response.InternalError(c, "Failed to suspend user")
	}

	return response.OK(c, fiber.Map{"message": "User suspended"})
}

// UnsuspendUser reactivates a user account
func (h *Handler) UnsuspendUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid user ID")
	}

	if err := h.service.UnsuspendUser(c.Context(), id); err != nil {
		return response.InternalError(c, "Failed to unsuspend user")
	}

	return response.OK(c, fiber.Map{"message": "User reactivated"})
}

// SystemHealth returns system health information
func (h *Handler) SystemHealth(c *fiber.Ctx) error {
	health, err := h.service.GetSystemHealth(c.Context())
	if err != nil {
		return response.InternalError(c, "Failed to get system health")
	}

	return c.JSON(health)
}
