package pipeline

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Pipeline defines the email send pipeline interface.
type Pipeline interface {
	Execute(ctx context.Context, state *State) (*Result, error)
}

// Stage represents a single stage in the pipeline.
type Stage interface {
	Execute(ctx context.Context, state *State) error
	Name() string
}

// EmailRepository defines the interface for email persistence (to avoid import cycle).
type EmailRepository interface {
	Create(ctx context.Context, model *EmailModel) error
}

// EmailModel represents an email log entry (to avoid import cycle).
type EmailModel struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DomainID       *uuid.UUID
	MessageID      string
	FromEmail      string
	ToEmail        string
	Subject        string
	Status         string
	TemplateID     *uuid.UUID
	Tags           []string
	MaxRetries     int
	IdempotencyKey *string
}

// State holds the data that flows through the pipeline.
type State struct {
	// Input
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

	// Processed data (populated by stages)
	RenderedSubject  string
	RenderedHTML     string
	RenderedText     string
	SanitizedHeaders map[string]string
	MessageID        string
	DomainID         *uuid.UUID
	EmailLogID       uuid.UUID
	CreditReserved   bool
	SpamScore        int
}

// AttachmentData represents attachment data in the pipeline.
type AttachmentData struct {
	Filename    string
	ContentType string
	Data        []byte
	Size        int64
}

// Result is the output of the pipeline.
type Result struct {
	EmailID   uuid.UUID
	MessageID string
	Status    string
}

// EmailSendPipeline implements the Pipeline interface.
type EmailSendPipeline struct {
	stages []Stage
}

// NewEmailSendPipeline creates a new email send pipeline.
func NewEmailSendPipeline(stages []Stage) Pipeline {
	return &EmailSendPipeline{
		stages: stages,
	}
}

// Execute runs all stages in sequence.
func (p *EmailSendPipeline) Execute(ctx context.Context, state *State) (*Result, error) {
	for _, stage := range p.stages {
		start := time.Now()

		if err := stage.Execute(ctx, state); err != nil {
			return nil, err
		}

		duration := time.Since(start)
		_ = duration // TODO: Add metrics
	}

	return &Result{
		EmailID:   state.EmailLogID,
		MessageID: state.MessageID,
		Status:    "queued",
	}, nil
}
