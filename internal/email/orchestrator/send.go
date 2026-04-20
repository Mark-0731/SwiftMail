package orchestrator

import (
	"context"

	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/email"
	emaildomain "github.com/Mark-0731/SwiftMail/internal/email/domain"
	"github.com/Mark-0731/SwiftMail/internal/email/pipeline"
	"github.com/Mark-0731/SwiftMail/internal/email/repository"
	"github.com/Mark-0731/SwiftMail/internal/events"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/cache"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/queue"
	"github.com/Mark-0731/SwiftMail/internal/template"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Orchestrator coordinates email sending through the pipeline.
// This is the single entry point for all email operations.
type Orchestrator struct {
	pipeline pipeline.Pipeline
	repo     repository.Repository
	logger   zerolog.Logger
}

// NewOrchestrator creates a new email orchestrator with dependency injection.
func NewOrchestrator(
	repo repository.Repository,
	templateSvc template.Service,
	cache cache.Cache,
	queue queue.Queue,
	rdb *redis.Client,
	logger zerolog.Logger,
) *Orchestrator {
	// Create event bus
	eventBus := events.NewRedisBus(rdb, logger)

	// Create domain services (pure business logic)
	spamDetector := emaildomain.NewSpamDetector()
	attachmentValidator := emaildomain.NewAttachmentValidator()
	contentSanitizer := emaildomain.NewContentSanitizer()
	deliverabilityValidator := emaildomain.NewDeliverabilityValidator(logger)
	creditService := billing.NewCreditService(cache, logger)

	// Create repository adapter for pipeline
	repoAdapter := &repositoryAdapter{repo: repo}

	// Build pipeline steps (dependency injection)
	steps := []pipeline.Step{
		pipeline.NewValidationStage(cache, creditService, logger),
		pipeline.NewRenderStage(templateSvc, logger),
		pipeline.NewSecurityStage(spamDetector, contentSanitizer, attachmentValidator, deliverabilityValidator, logger),
		pipeline.NewPersistenceStage(repoAdapter, cache, creditService, logger),
		pipeline.NewDispatchStage(queue, eventBus, logger),
	}

	// Create pipeline executor
	emailPipeline := pipeline.NewEmailSendPipeline(steps)

	return &Orchestrator{
		pipeline: emailPipeline,
		repo:     repo,
		logger:   logger,
	}
}

// repositoryAdapter adapts repository.Repository to pipeline.EmailRepository.
type repositoryAdapter struct {
	repo repository.Repository
}

func (a *repositoryAdapter) Create(ctx context.Context, model *pipeline.EmailModel) error {
	emailModel := &email.Model{
		ID:             model.ID,
		UserID:         model.UserID,
		DomainID:       model.DomainID,
		MessageID:      model.MessageID,
		FromEmail:      model.FromEmail,
		ToEmail:        model.ToEmail,
		Subject:        model.Subject,
		Status:         model.Status,
		TemplateID:     model.TemplateID,
		Tags:           model.Tags,
		MaxRetries:     model.MaxRetries,
		IdempotencyKey: model.IdempotencyKey,
	}
	return a.repo.Create(ctx, emailModel)
}

// Send orchestrates email sending through the pipeline.
// This is the primary entry point for sending emails.
func (o *Orchestrator) Send(ctx context.Context, userID uuid.UUID, req *email.SendRequest, idempotencyKey string) (*email.SendResponse, error) {
	// Convert attachments to pipeline format
	pipelineAttachments := make([]pipeline.AttachmentData, len(req.Attachments))
	for i, att := range req.Attachments {
		pipelineAttachments[i] = pipeline.AttachmentData{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Data:        att.Data,
			Size:        att.Size,
		}
	}

	// Create email context (shared state for all pipeline steps)
	emailCtx := pipeline.NewEmailContext(ctx, userID, req.From, req.To)
	emailCtx.Subject = req.Subject
	emailCtx.HTML = req.HTML
	emailCtx.Text = req.Text
	emailCtx.ReplyTo = &req.ReplyTo
	emailCtx.Headers = req.Headers
	emailCtx.TemplateID = req.TemplateID
	emailCtx.Variables = req.Variables
	emailCtx.Tags = req.Tags
	emailCtx.Attachments = pipelineAttachments
	emailCtx.IdempotencyKey = idempotencyKey

	// Execute pipeline
	result, err := o.pipeline.Execute(ctx, emailCtx)
	if err != nil {
		return nil, err
	}

	// Return response
	return &email.SendResponse{
		ID:        result.EmailID,
		MessageID: result.MessageID,
		Status:    result.Status,
	}, nil
}

// GetLog retrieves an email log by ID.
func (o *Orchestrator) GetLog(ctx context.Context, id uuid.UUID) (*email.Model, error) {
	return o.repo.GetByID(ctx, id)
}

// SearchLogs searches email logs with filters.
func (o *Orchestrator) SearchLogs(ctx context.Context, q *email.LogQuery) ([]email.Model, int64, error) {
	return o.repo.Search(ctx, q)
}
