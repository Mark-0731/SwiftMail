package email

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/swiftmail/swiftmail/internal/server/middleware"
	"github.com/swiftmail/swiftmail/pkg/response"
)

// Handler holds email HTTP handlers.
type Handler struct {
	service Service
}

// NewHandler creates email handlers.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// Send handles POST /v1/mail/send — the critical hot path.
func (h *Handler) Send(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	var req SendRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.To == "" || req.From == "" {
		return response.ValidationError(c, "to and from are required")
	}

	if req.Subject == "" && req.TemplateID == nil {
		return response.ValidationError(c, "subject is required when not using a template")
	}

	idempotencyKey := c.Get("Idempotency-Key")

	resp, err := h.service.Send(c.Context(), userID, &req, idempotencyKey)
	if err != nil {
		// Log the actual error for debugging
		c.Locals("error", err.Error())

		errMsg := err.Error()
		switch {
		case contains(errMsg, "suppressed"):
			return response.BadRequest(c, "SUPPRESSED", errMsg)
		case contains(errMsg, "insufficient credits"):
			return response.BadRequest(c, "INSUFFICIENT_CREDITS", "Not enough credits to send email")
		case contains(errMsg, "invalid"):
			return response.ValidationError(c, errMsg)
		default:
			return response.InternalError(c, "Failed to queue email: "+errMsg)
		}
	}

	return response.Accepted(c, resp)
}

func (h *Handler) GetLog(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid log ID")
	}

	log, err := h.service.GetLog(c.Context(), id)
	if err != nil {
		return response.NotFound(c, "Email log not found")
	}

	return response.OK(c, log)
}

func (h *Handler) SearchLogs(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))

	q := &LogQuery{
		UserID:   userID,
		Email:    c.Query("email"),
		Domain:   c.Query("domain"),
		Tag:      c.Query("tag"),
		Status:   c.Query("status"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
		Page:     page,
		PerPage:  perPage,
	}

	logs, total, err := h.service.SearchLogs(c.Context(), q)
	if err != nil {
		return response.InternalError(c, "Failed to search logs")
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return response.Paginated(c, logs, response.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
