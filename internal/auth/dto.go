package auth

import (
	"time"

	"github.com/google/uuid"
)

// ===== Request DTOs =====

type SignupRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Name     string `json:"name" validate:"required,min=2"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	TOTPCode string `json:"totp_code,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type TOTPSetupRequest struct{}

type TOTPVerifyRequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type CreateAPIKeyRequest struct {
	Name        string     `json:"name" validate:"required"`
	Permissions []string   `json:"permissions,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// ===== Response DTOs =====

type AuthResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
}

type UserResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	EmailVerified bool      `json:"email_verified"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type TOTPSetupResponse struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
	QRCode string `json:"qr_code"`
}

type APIKeyResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Key         string     `json:"key,omitempty"` // Only shown once at creation
	Permissions []string   `json:"permissions"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type APIKeyListResponse struct {
	Keys []APIKeyResponse `json:"keys"`
}
type PasswordResetRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type PasswordResetConfirmRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type PasswordResetResponse struct {
	Message string `json:"message"`
}

type EmailVerificationRequest struct {
	Token string `json:"token" validate:"required"`
}

type EmailVerificationResponse struct {
	Message string `json:"message"`
}

type ResendVerificationRequest struct{}

type ResendVerificationResponse struct {
	Message string `json:"message"`
}

type RevokeSessionRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RevokeAllSessionsRequest struct{}

type SessionResponse struct {
	Message string `json:"message"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

type ChangePasswordResponse struct {
	Message string `json:"message"`
}
