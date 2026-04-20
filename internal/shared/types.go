package shared

import (
	"github.com/google/uuid"
)

// Task type constants
const (
	TaskEmailSend       = "email:send"
	TaskEmailBulk       = "email:send:bulk"
	TaskTrackingEvent   = "tracking:event"
	TaskBounceProcess   = "bounce:process"
	TaskWebhookDispatch = "webhook:dispatch"
)

// TrackingEventPayload is the payload for tracking events.
type TrackingEventPayload struct {
	EmailLogID uuid.UUID         `json:"email_log_id"`
	EventType  string            `json:"event_type"` // opened, clicked
	IPAddress  string            `json:"ip_address"`
	UserAgent  string            `json:"user_agent"`
	URL        string            `json:"url,omitempty"` // For click events
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// BouncePayload is the payload for bounce processing.
type BouncePayload struct {
	EmailLogID uuid.UUID `json:"email_log_id"`
	BounceType string    `json:"bounce_type"` // hard, soft, complaint
	Recipient  string    `json:"recipient"`
	Diagnostic string    `json:"diagnostic"`
}

// EmailSendPayload is the payload for email sending tasks.
type EmailSendPayload struct {
	EmailLogID uuid.UUID         `json:"email_log_id"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	HTML       string            `json:"html"`
	Text       string            `json:"text"`
	ReplyTo    string            `json:"reply_to,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	MessageID  string            `json:"message_id"`
	UserID     string            `json:"user_id"`
}
