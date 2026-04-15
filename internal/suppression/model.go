package suppression

import (
	"time"

	"github.com/google/uuid"
)

// Entry represents a suppression list entry.
type Entry struct {
	ID        uuid.UUID  `json:"id"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	Email     string     `json:"email"`
	Type      string     `json:"type"`     // hard_bounce, soft_bounce, complaint, unsubscribe, manual
	Reason    *string    `json:"reason,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// AddRequest is the DTO for adding a suppression entry.
type AddRequest struct {
	Email  string `json:"email" validate:"required,email"`
	Type   string `json:"type" validate:"required,oneof=hard_bounce soft_bounce complaint unsubscribe manual"`
	Reason string `json:"reason"`
}
