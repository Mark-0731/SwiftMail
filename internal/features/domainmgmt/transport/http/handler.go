package http

import (
	"errors"

	domainmgmt "github.com/Mark-0731/SwiftMail/internal/features/domainmgmt"
	"github.com/Mark-0731/SwiftMail/internal/features/domainmgmt/application"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Handler holds domain HTTP handlers.
type Handler struct {
	service application.Service
}

// NewHandler creates domain handlers.
func NewHandler(service application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) AddDomain(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	var req domainmgmt.AddDomainRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}
	if req.Domain == "" {
		return response.ValidationError(c, "domain is required")
	}

	resp, err := h.service.AddDomain(c.Context(), userID, &req)
	if err != nil {
		return response.InternalError(c, err.Error())
	}

	return response.Created(c, resp)
}

func (h *Handler) ListDomains(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	resp, err := h.service.ListDomains(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to list domains")
	}
	return response.OK(c, resp)
}

func (h *Handler) GetDomain(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid domain ID")
	}

	resp, err := h.service.GetDomain(c.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return response.NotFound(c, "Domain not found")
		}
		return response.InternalError(c, "Failed to get domain")
	}

	return response.OK(c, resp)
}

func (h *Handler) VerifyDomain(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid domain ID")
	}

	resp, err := h.service.VerifyDomain(c.Context(), id)
	if err != nil {
		return response.InternalError(c, "Verification failed")
	}

	return response.OK(c, resp)
}

func (h *Handler) DeleteDomain(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid domain ID")
	}

	if err := h.service.DeleteDomain(c.Context(), id, userID); err != nil {
		return response.InternalError(c, "Failed to delete domain")
	}

	return response.OK(c, map[string]string{"message": "Domain deleted"})
}
