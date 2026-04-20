package events

import (
	"time"

	"github.com/google/uuid"
)

// Event types
const (
	EmailQueued  = "email.queued"
	EmailSent    = "email.sent"
	EmailFailed  = "email.failed"
	EmailOpened  = "email.opened"
	EmailClicked = "email.clicked"
	EmailBounced = "email.bounced"
)

// Event represents a domain event.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// NewEvent creates a new event.
func NewEvent(eventType string, data map[string]interface{}) *Event {
	return &Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// EmailQueuedEvent creates an email queued event.
func EmailQueuedEvent(emailID, userID uuid.UUID) *Event {
	return NewEvent(EmailQueued, map[string]interface{}{
		"email_id": emailID.String(),
		"user_id":  userID.String(),
	})
}

// EmailSentEvent creates an email sent event.
func EmailSentEvent(emailID, userID uuid.UUID, to string) *Event {
	return NewEvent(EmailSent, map[string]interface{}{
		"email_id": emailID.String(),
		"user_id":  userID.String(),
		"to":       to,
	})
}

// EmailFailedEvent creates an email failed event.
func EmailFailedEvent(emailID, userID uuid.UUID, reason string) *Event {
	return NewEvent(EmailFailed, map[string]interface{}{
		"email_id": emailID.String(),
		"user_id":  userID.String(),
		"reason":   reason,
	})
}
