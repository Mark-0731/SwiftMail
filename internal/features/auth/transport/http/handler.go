package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/Mark-0731/SwiftMail/internal/features/auth"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/application"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds auth HTTP handlers.
type Handler struct {
	service application.Service
}

// NewHandler creates auth handlers.
func NewHandler(service application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Signup(c *fiber.Ctx) error {
	var req auth.SignupRequest
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
		if errors.Is(err, application.ErrUserExists) {
			return response.Conflict(c, "User with this email already exists")
		}
		return response.InternalError(c, "Failed to create account")
	}

	return response.Created(c, resp)
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req auth.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	resp, err := h.service.Login(c.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidCredentials):
			return response.Unauthorized(c, "Invalid email or password")
		case errors.Is(err, application.ErrUserSuspended):
			return response.Forbidden(c, "Account is suspended")
		case errors.Is(err, application.ErrTOTPRequired):
			return response.BadRequest(c, "TOTP_REQUIRED", "2FA code required")
		case errors.Is(err, application.ErrTOTPInvalid):
			return response.Unauthorized(c, "Invalid 2FA code")
		default:
			return response.InternalError(c, "Failed to login")
		}
	}

	return response.OK(c, resp)
}

func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var req auth.RefreshRequest
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

	var req auth.TOTPVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if err := h.service.VerifyTOTP(c.Context(), userID, req.Code); err != nil {
		if errors.Is(err, application.ErrTOTPInvalid) {
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

	var req auth.CreateAPIKeyRequest
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

func (h *Handler) RequestPasswordReset(c *fiber.Ctx) error {
	var req auth.PasswordResetRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Email == "" {
		return response.ValidationError(c, "email is required")
	}

	if err := h.service.RequestPasswordReset(c.Context(), req.Email); err != nil {
		if errors.Is(err, application.ErrRateLimitExceeded) {
			return response.TooManyRequests(c, "Too many reset requests. Please try again later.")
		}
		// Don't reveal if email exists - always return success
	}

	return response.OK(c, auth.PasswordResetResponse{
		Message: "If the email exists, a password reset link has been sent",
	})
}

func (h *Handler) ResetPassword(c *fiber.Ctx) error {
	var req auth.PasswordResetConfirmRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Token == "" || req.NewPassword == "" {
		return response.ValidationError(c, "token and new_password are required")
	}

	if len(req.NewPassword) < 8 {
		return response.ValidationError(c, "password must be at least 8 characters")
	}

	if err := h.service.ResetPassword(c.Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, application.ErrInvalidToken) {
			return response.BadRequest(c, "INVALID_TOKEN", "Invalid or expired reset token")
		}
		return response.InternalError(c, "Failed to reset password")
	}

	return response.OK(c, auth.PasswordResetResponse{
		Message: "Password reset successful. Please login with your new password.",
	})
}

func (h *Handler) VerifyEmail(c *fiber.Ctx) error {
	var req auth.EmailVerificationRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Token == "" {
		return response.ValidationError(c, "token is required")
	}

	if err := h.service.VerifyEmail(c.Context(), req.Token); err != nil {
		if errors.Is(err, application.ErrInvalidToken) {
			return response.BadRequest(c, "INVALID_TOKEN", "Invalid or expired verification token")
		}
		return response.InternalError(c, "Failed to verify email")
	}

	return response.OK(c, auth.EmailVerificationResponse{
		Message: "Email verified successfully",
	})
}

func (h *Handler) ResendVerificationEmail(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	if err := h.service.SendVerificationEmail(c.Context(), userID); err != nil {
		if errors.Is(err, application.ErrRateLimitExceeded) {
			return response.TooManyRequests(c, "Too many verification requests. Please try again later.")
		}
		return response.InternalError(c, "Failed to send verification email")
	}

	return response.OK(c, auth.ResendVerificationResponse{
		Message: "Verification email sent",
	})
}

func (h *Handler) RevokeSession(c *fiber.Ctx) error {
	var req auth.RevokeSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.RefreshToken == "" {
		return response.ValidationError(c, "refresh_token is required")
	}

	if err := h.service.RevokeSession(c.Context(), req.RefreshToken); err != nil {
		if errors.Is(err, application.ErrInvalidToken) {
			return response.BadRequest(c, "INVALID_TOKEN", "Invalid refresh token")
		}
		return response.InternalError(c, "Failed to revoke session")
	}

	return response.OK(c, auth.SessionResponse{
		Message: "Session revoked successfully",
	})
}

func (h *Handler) RevokeAllSessions(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	if err := h.service.RevokeAllSessions(c.Context(), userID); err != nil {
		return response.InternalError(c, "Failed to revoke sessions")
	}

	return response.OK(c, auth.SessionResponse{
		Message: "All sessions revoked successfully",
	})
}

func (h *Handler) ChangePassword(c *fiber.Ctx) error {
	userID := getUserID(c)
	if userID == uuid.Nil {
		return response.Unauthorized(c, "Authentication required")
	}

	var req auth.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return response.ValidationError(c, "current_password and new_password are required")
	}

	if len(req.NewPassword) < 8 {
		return response.ValidationError(c, "new password must be at least 8 characters")
	}

	if err := h.service.ChangePassword(c.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, application.ErrInvalidCredentials) {
			return response.Unauthorized(c, "Current password is incorrect")
		}
		return response.InternalError(c, "Failed to change password")
	}

	return response.OK(c, auth.ChangePasswordResponse{
		Message: "Password changed successfully. All sessions have been revoked.",
	})
}
