package webhook

import (
	"time"

	"github.com/google/uuid"
)

// Config represents a user's webhook configuration.
type Config struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	URL       string    `json:"url"`
	Secret    string    `json:"-"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRequest is the DTO for creating a webhook.
type CreateRequest struct {
	URL    string   `json:"url" validate:"required,url"`
	Events []string `json:"events" validate:"required,min=1"`
}

// Log represents a webhook delivery attempt.
type Log struct {
	ID             uuid.UUID `json:"id"`
	WebhookID      uuid.UUID `json:"webhook_id"`
	EventType      string    `json:"event_type"`
	ResponseStatus int       `json:"response_status"`
	Success        bool      `json:"success"`
	CreatedAt      time.Time `json:"created_at"`
}
