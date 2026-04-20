package email

import (
	"github.com/google/uuid"
)

// SendRequest is the request body for POST /v1/mail/send.
type SendRequest struct {
	To          string            `json:"to" validate:"required,email"`
	From        string            `json:"from" validate:"required,email"`
	Subject     string            `json:"subject" validate:"required"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	TemplateID  *uuid.UUID        `json:"template_id,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []AttachmentData  `json:"attachments,omitempty"`
}

// AttachmentData represents attachment data in the request.
type AttachmentData struct {
	Filename    string `json:"filename" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	Data        []byte `json:"data" validate:"required"` // Base64 encoded data
	Size        int64  `json:"size"`                     // Size in bytes
}

// SendResponse is the response for POST /v1/mail/send.
type SendResponse struct {
	ID        uuid.UUID `json:"id"`
	MessageID string    `json:"message_id"`
	Status    string    `json:"status"`
}

// LogQuery represents search filters for email logs.
type LogQuery struct {
	UserID   uuid.UUID
	Email    string
	Domain   string
	Tag      string
	Status   string
	DateFrom string
	DateTo   string
	Page     int
	PerPage  int
}

// LogResponse is the response for an email log entry.
type LogResponse struct {
	ID           uuid.UUID       `json:"id"`
	MessageID    string          `json:"message_id"`
	FromEmail    string          `json:"from_email"`
	ToEmail      string          `json:"to_email"`
	Subject      string          `json:"subject"`
	Status       string          `json:"status"`
	Tags         []string        `json:"tags"`
	IPUsed       *string         `json:"ip_used"`
	SMTPResponse *string         `json:"smtp_response"`
	RetryCount   int             `json:"retry_count"`
	Attachments  []AttachmentRef `json:"attachments"`
	OpenedAt     *string         `json:"opened_at"`
	ClickedAt    *string         `json:"clicked_at"`
	BouncedAt    *string         `json:"bounced_at"`
	CreatedAt    string          `json:"created_at"`
}
