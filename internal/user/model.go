package user

import (
	"time"

	"github.com/google/uuid"
)

// User represents a platform user.
type User struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	PasswordHash  string     `json:"-"`
	Role          string     `json:"role"`     // owner, admin, member
	Status        string     `json:"status"`   // active, suspended, deleted
	TOTPEnabled   bool       `json:"totp_enabled"`
	TOTPSecret    *string    `json:"-"`
	EmailVerified bool       `json:"email_verified"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	SuspendedAt   *time.Time `json:"suspended_at,omitempty"`
}

// ProfileResponse is the public profile DTO.
type ProfileResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateProfileRequest is the DTO for updating a user's profile.
type UpdateProfileRequest struct {
	Name string `json:"name" validate:"required,min=1"`
}
