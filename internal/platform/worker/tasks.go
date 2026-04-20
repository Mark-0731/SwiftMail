package worker
import (
	"encoding/json"
	"github.com/google/uuid"
)
// Task type constants
const (
	TaskEmailSend     = "email:send"
	TaskEmailBulk     = "email:send:bulk"
	TaskTrackingEvent = "tracking:event"
	TaskBounceProcess = "bounce:process"
	TaskWebhookDispatch = "webhook:dispatch"
)
// EmailSendPayload is the payload for the email:send task.
type EmailSendPayload struct {
	EmailLogID uuid.UUID         `json:"email_log_id"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	HTML       string            `json:"html"`
	Text       string            `json:"text"`
	ReplyTo    string            `json:"reply_to"`
	Headers    map[string]string `json:"headers"`
	MessageID  string            `json:"message_id"`
	UserID     uuid.UUID         `json:"user_id"`
}
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
	EmailLogID   uuid.UUID `json:"email_log_id"`
	BounceType   string    `json:"bounce_type"` // hard, soft
	BounceCode   string    `json:"bounce_code"`
	Diagnostic   string    `json:"diagnostic"`
	Recipient    string    `json:"recipient"`
	UserID       uuid.UUID `json:"user_id"`
}
// WebhookPayload is the payload for webhook dispatch.
type WebhookPayload struct {
	WebhookID  uuid.UUID              `json:"webhook_id"`
	EventType  string                 `json:"event_type"`
	EventData  map[string]interface{} `json:"event_data"`
}
// MarshalPayload serializes a payload to JSON bytes.
func MarshalPayload(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
// UnmarshalPayload deserializes JSON bytes to a payload struct.
func UnmarshalPayload(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
