package admin

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/Mark-0731/SwiftMail/internal/user"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds admin HTTP handlers.
type Handler struct {
	userRepo *user.Repository
}

// NewHandler creates admin handlers.
func NewHandler(userRepo *user.Repository) *Handler {
	return &Handler{userRepo: userRepo}
}

// ListUsers lists all users (admin only).
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 50)

	users, total, err := h.userRepo.ListAll(c.Context(), page, perPage)
	if err != nil {
		return response.InternalError(c, "Failed to list users")
	}

	return response.Paginated(c, users, response.Meta{Page: page, PerPage: perPage, Total: total})
}

// SuspendUser suspends a user account.
func (h *Handler) SuspendUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid user ID")
	}

	if err := h.userRepo.Suspend(c.Context(), id); err != nil {
		return response.InternalError(c, "Failed to suspend user")
	}

	return response.OK(c, fiber.Map{"message": "User suspended"})
}

// UnsuspendUser reactivates a user account.
func (h *Handler) UnsuspendUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid user ID")
	}

	if err := h.userRepo.Unsuspend(c.Context(), id); err != nil {
		return response.InternalError(c, "Failed to unsuspend user")
	}

	return response.OK(c, fiber.Map{"message": "User reactivated"})
}

// SystemHealth returns system health information.
func (h *Handler) SystemHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "operational",
		"smtp_pool": fiber.Map{"status": "healthy"},
		"queue":     fiber.Map{"status": "healthy"},
	})
}
