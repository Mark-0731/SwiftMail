package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Pipeline defines the email send pipeline interface.
type Pipeline interface {
	Execute(ctx context.Context, emailCtx *EmailContext) (*Result, error)
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

// Result is the output of the pipeline.
type Result struct {
	EmailID   uuid.UUID
	MessageID string
	Status    string
	Duration  time.Duration
	StepTimes map[string]time.Duration
}

// EmailSendPipeline implements the Pipeline interface.
type EmailSendPipeline struct {
	steps []Step
}

// NewEmailSendPipeline creates a new email send pipeline.
func NewEmailSendPipeline(steps []Step) Pipeline {
	return &EmailSendPipeline{
		steps: steps,
	}
}

// Execute runs all steps in sequence with metrics tracking.
func (p *EmailSendPipeline) Execute(ctx context.Context, emailCtx *EmailContext) (*Result, error) {
	for _, step := range p.steps {
		start := time.Now()

		err := step.Execute(ctx, emailCtx)
		duration := time.Since(start)

		// Record step duration in context
		emailCtx.RecordStepDuration(step.Name(), duration)

		if err != nil {
			return nil, fmt.Errorf("pipeline step '%s' failed: %w", step.Name(), err)
		}
	}

	return &Result{
		EmailID:   emailCtx.EmailLogID,
		MessageID: emailCtx.MessageID,
		Status:    "queued",
		Duration:  emailCtx.TotalDuration(),
		StepTimes: emailCtx.StepTimes,
	}, nil
}
