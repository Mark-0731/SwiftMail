package template

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds template HTTP handlers.
type Handler struct {
	service Service
}

// NewHandler creates template handlers.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	var req CreateTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}
	if req.Name == "" || req.Subject == "" {
		return response.ValidationError(c, "name and subject are required")
	}
	t, err := h.service.Create(c.Context(), userID, &req)
	if err != nil {
		return response.InternalError(c, "Failed to create template")
	}
	return response.Created(c, t)
}

func (h *Handler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	templates, err := h.service.List(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to list templates")
	}
	return response.OK(c, templates)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid template ID")
	}
	t, err := h.service.Get(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "Template not found")
	}
	return response.OK(c, t)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid template ID")
	}
	var req UpdateTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}
	t, err := h.service.Update(c.Context(), id, &req)
	if err != nil {
		return response.InternalError(c, "Failed to update template")
	}
	return response.OK(c, t)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid template ID")
	}
	if err := h.service.Delete(c.Context(), id, userID); err != nil {
		return response.InternalError(c, "Failed to delete template")
	}
	return response.OK(c, map[string]string{"message": "Template deleted"})
}

func (h *Handler) Preview(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid template ID")
	}
	var req PreviewRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}
	subject, html, text, err := h.service.Preview(c.Context(), id, req.Variables)
	if err != nil {
		return response.InternalError(c, "Failed to preview template")
	}
	return response.OK(c, map[string]string{"subject": subject, "html": html, "text": text})
}

func (h *Handler) Duplicate(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid template ID")
	}
	t, err := h.service.Duplicate(c.Context(), id, userID)
	if err != nil {
		return response.InternalError(c, "Failed to duplicate template")
	}
	return response.Created(c, t)
}

func (h *Handler) Archive(c *fiber.Ctx) error {
	id, _ := uuid.Parse(c.Params("id"))
	if err := h.service.Archive(c.Context(), id); err != nil {
		return response.InternalError(c, "Failed to archive template")
	}
	return response.OK(c, map[string]string{"message": "Template archived"})
}

func (h *Handler) Restore(c *fiber.Ctx) error {
	id, _ := uuid.Parse(c.Params("id"))
	if err := h.service.Restore(c.Context(), id); err != nil {
		return response.InternalError(c, "Failed to restore template")
	}
	return response.OK(c, map[string]string{"message": "Template restored"})
}

func (h *Handler) GetVersions(c *fiber.Ctx) error {
	id, _ := uuid.Parse(c.Params("id"))
	versions, err := h.service.GetVersions(c.Context(), id)
	if err != nil {
		return response.InternalError(c, "Failed to get versions")
	}
	return response.OK(c, versions)
}

func (h *Handler) Rollback(c *fiber.Ctx) error {
	id, _ := uuid.Parse(c.Params("id"))
	version := c.QueryInt("version", 0)
	if version == 0 {
		return response.BadRequest(c, "INVALID_VERSION", "Version number required")
	}
	if err := h.service.Rollback(c.Context(), id, version); err != nil {
		return response.InternalError(c, "Failed to rollback")
	}
	return response.OK(c, map[string]string{"message": "Rolled back successfully"})
}
