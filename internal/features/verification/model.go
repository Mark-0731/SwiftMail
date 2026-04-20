package verification

import (
	"time"

	"github.com/google/uuid"
)

// VerificationToken represents an email verification token
type VerificationToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// SendVerificationEmailRequest is the request to send verification email
type SendVerificationEmailRequest struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Name   string    `json:"name"`
}

// VerifyEmailRequest is the request to verify email
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// VerificationResponse is the response for verification operations
type VerificationResponse struct {
	Message string `json:"message"`
}
