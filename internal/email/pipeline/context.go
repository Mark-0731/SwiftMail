package pipeline

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EmailContext holds all data that flows through the pipeline.
// This is the shared context object that all steps operate on.
// It provides methods for state management and validation.
type EmailContext struct {
	// Context for cancellation and deadlines
	ctx context.Context

	// Input (immutable after creation)
	UserID         uuid.UUID
	From           string
	To             string
	Subject        string
	HTML           string
	Text           string
	ReplyTo        *string
	Headers        map[string]string
	TemplateID     *uuid.UUID
	Variables      map[string]string
	Tags           []string
	Attachments    []AttachmentData
	IdempotencyKey string

	// Processed data (populated by pipeline steps)
	RenderedSubject  string
	RenderedHTML     string
	RenderedText     string
	SanitizedHeaders map[string]string
	MessageID        string
	DomainID         *uuid.UUID
	EmailLogID       uuid.UUID
	CreditReserved   bool
	SpamScore        int

	// Metadata
	StartTime time.Time
	StepTimes map[string]time.Duration
	Metadata  map[string]interface{}
}

// State is an alias for EmailContext (backward compatibility).
type State = EmailContext

// AttachmentData represents attachment data in the pipeline.
type AttachmentData struct {
	Filename    string
	ContentType string
	Data        []byte
	Size        int64
}

// NewEmailContext creates a new email context with defaults.
func NewEmailContext(ctx context.Context, userID uuid.UUID, from, to string) *EmailContext {
	return &EmailContext{
		ctx:       ctx,
		UserID:    userID,
		From:      from,
		To:        to,
		StartTime: time.Now(),
		StepTimes: make(map[string]time.Duration),
		Metadata:  make(map[string]interface{}),
	}
}

// Context returns the underlying context for cancellation/deadlines.
func (ec *EmailContext) Context() context.Context {
	return ec.ctx
}

// RecordStepDuration records the duration of a step execution.
func (ec *EmailContext) RecordStepDuration(stepName string, duration time.Duration) {
	ec.StepTimes[stepName] = duration
}

// TotalDuration returns the total time elapsed since context creation.
func (ec *EmailContext) TotalDuration() time.Duration {
	return time.Since(ec.StartTime)
}

// SetMetadata sets a metadata value.
func (ec *EmailContext) SetMetadata(key string, value interface{}) {
	ec.Metadata[key] = value
}

// GetMetadata retrieves a metadata value.
func (ec *EmailContext) GetMetadata(key string) (interface{}, bool) {
	val, ok := ec.Metadata[key]
	return val, ok
}

// HasRenderedContent checks if content has been rendered.
func (ec *EmailContext) HasRenderedContent() bool {
	return ec.RenderedSubject != "" || ec.RenderedHTML != "" || ec.RenderedText != ""
}

// GetEffectiveSubject returns rendered subject or original.
func (ec *EmailContext) GetEffectiveSubject() string {
	if ec.RenderedSubject != "" {
		return ec.RenderedSubject
	}
	return ec.Subject
}

// GetEffectiveHTML returns rendered HTML or original.
func (ec *EmailContext) GetEffectiveHTML() string {
	if ec.RenderedHTML != "" {
		return ec.RenderedHTML
	}
	return ec.HTML
}

// GetEffectiveText returns rendered text or original.
func (ec *EmailContext) GetEffectiveText() string {
	if ec.RenderedText != "" {
		return ec.RenderedText
	}
	return ec.Text
}

// GetEffectiveHeaders returns sanitized headers or original.
func (ec *EmailContext) GetEffectiveHeaders() map[string]string {
	if ec.SanitizedHeaders != nil {
		return ec.SanitizedHeaders
	}
	return ec.Headers
}
