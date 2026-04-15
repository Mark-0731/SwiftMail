package email

import (
	"time"

	"github.com/google/uuid"
)

// Status constants for the email delivery state machine.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSending    = "sending"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusDeferred   = "deferred"
	StatusFailed     = "failed"
	StatusBounced    = "bounced"
	StatusComplained = "complained"
)

// ValidTransitions defines allowed state transitions for the email state machine.
var ValidTransitions = map[string][]string{
	StatusQueued:     {StatusProcessing},
	StatusProcessing: {StatusSending, StatusFailed},
	StatusSending:    {StatusSent, StatusDeferred, StatusFailed},
	StatusDeferred:   {StatusSending, StatusFailed},
	StatusSent:       {StatusDelivered, StatusBounced, StatusComplained},
	StatusDelivered:  {}, // Terminal (can still get opened/clicked events)
	StatusFailed:     {}, // Terminal
	StatusBounced:    {}, // Terminal
	StatusComplained: {}, // Terminal
}

// CanTransition checks if a state transition is valid.
func CanTransition(from, to string) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// Model represents an email log record.
type Model struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	DomainID        uuid.UUID  `json:"domain_id"`
	IdempotencyKey  *string    `json:"idempotency_key,omitempty"`
	MessageID       string     `json:"message_id"`
	FromEmail       string     `json:"from_email"`
	ToEmail         string     `json:"to_email"`
	Subject         string     `json:"subject"`
	Status          string     `json:"status"`
	PreviousStatus  *string    `json:"previous_status,omitempty"`
	StatusChangedAt time.Time  `json:"status_changed_at"`
	TemplateID      *uuid.UUID `json:"template_id,omitempty"`
	Tags            []string   `json:"tags"`
	IPUsed          *string    `json:"ip_used,omitempty"`
	SMTPResponse    *string    `json:"smtp_response,omitempty"`
	RetryCount      int        `json:"retry_count"`
	MaxRetries      int        `json:"max_retries"`
	Attachments     []AttachmentRef `json:"attachments"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	OpenedAt        *time.Time `json:"opened_at,omitempty"`
	ClickedAt       *time.Time `json:"clicked_at,omitempty"`
	BouncedAt       *time.Time `json:"bounced_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AttachmentRef is a reference to a file stored in MinIO/S3.
type AttachmentRef struct {
	Key         string `json:"key"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}
