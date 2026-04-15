package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/swiftmail/swiftmail/pkg/response"
)

// Handler holds auth HTTP handlers.
type Handler struct {
	service Service
}

// NewHandler creates auth handlers.
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Signup(c *fiber.Ctx) error {
	var req SignupRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		return response.ValidationError(c, "email, password, and name are required")
	}

	if len(req.Password) < 8 {
		return response.ValidationError(c, "password must be at least 8 characters")
	}

	resp, err := h.service.Signup(c.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			return response.Conflict(c, "User with this email already exists")
		}
		return response.InternalError(c, "Failed to create account")
	}

	return response.Created(c, resp)
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	resp, err := h.service.Login(c.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			return response.Unauthorized(c, "Invalid email or password")
		case errors.Is(err, ErrUserSuspended):
			return response.Forbidden(c, "Account is suspended")
		case errors.Is(err, ErrTOTPRequired):
			return response.BadRequest(c, "TOTP_REQUIRED", "2FA code required")
		case errors.Is(err, ErrTOTPInvalid):
			return response.Unauthorized(c, "Invalid 2FA code")
		default:
			return response.InternalError(c, "Failed to login")
		}
	}

	return response.OK(c, resp)
}

func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var req RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	resp, err := h.service.RefreshToken(c.Context(), req.RefreshToken)
	if err != nil {
		return response.Unauthorized(c, "Invalid or expired refresh token")
	}

	return response.OK(c, resp)
}

func (h *Handler) GetProfile(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	resp, err := h.service.GetProfile(c.Context(), userID)
	if err != nil {
		return response.NotFound(c, "User not found")
	}

	return response.OK(c, resp)
}

func (h *Handler) SetupTOTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	resp, err := h.service.SetupTOTP(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to setup 2FA")
	}

	return response.OK(c, resp)
}

func (h *Handler) VerifyTOTP(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	var req TOTPVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if err := h.service.VerifyTOTP(c.Context(), userID, req.Code); err != nil {
		if errors.Is(err, ErrTOTPInvalid) {
			return response.BadRequest(c, "INVALID_CODE", "Invalid 2FA code")
		}
		return response.InternalError(c, "Failed to verify 2FA")
	}

	return response.OK(c, map[string]string{"message": "2FA enabled successfully"})
}

func (h *Handler) CreateAPIKey(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	var req CreateAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Name == "" {
		return response.ValidationError(c, "name is required")
	}

	resp, err := h.service.CreateAPIKey(c.Context(), userID, &req)
	if err != nil {
		return response.InternalError(c, "Failed to create API key")
	}

	return response.Created(c, resp)
}

func (h *Handler) ListAPIKeys(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	resp, err := h.service.ListAPIKeys(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "Failed to list API keys")
	}

	return response.OK(c, resp)
}

func (h *Handler) DeleteAPIKey(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	keyID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "INVALID_ID", "Invalid API key ID")
	}

	if err := h.service.DeleteAPIKey(c.Context(), keyID, userID); err != nil {
		return response.InternalError(c, "Failed to delete API key")
	}

	return response.OK(c, map[string]string{"message": "API key deleted"})
}

// getUserID extracts the user ID from Fiber context (set by auth middleware).
func getUserID(c *fiber.Ctx) uuid.UUID {
	id, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}
