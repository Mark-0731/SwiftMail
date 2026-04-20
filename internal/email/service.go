package email

import (
	"context"

	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/email/pipeline"
	emailservice "github.com/Mark-0731/SwiftMail/internal/email/service"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/cache"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/queue"
	"github.com/Mark-0731/SwiftMail/internal/template"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Service defines the email business logic interface.
type Service interface {
	Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error)
	GetLog(ctx context.Context, id uuid.UUID) (*Model, error)
	SearchLogs(ctx context.Context, q *LogQuery) ([]Model, int64, error)
}

type service struct {
	pipeline pipeline.Pipeline
	repo     Repository
	logger   zerolog.Logger
}

// NewService creates a new email service with pipeline architecture.
func NewService(
	repo Repository,
	templateSvc template.Service,
	cache cache.Cache,
	queue queue.Queue,
	logger zerolog.Logger,
) Service {
	// Create domain services
	spamDetector := emailservice.NewSpamDetector()
	attachmentValidator := emailservice.NewAttachmentValidator()
	contentSanitizer := emailservice.NewContentSanitizer()
	deliverabilityValidator := emailservice.NewDeliverabilityValidator(logger)
	creditService := billing.NewCreditService(cache, logger)

	// Create repository adapter for pipeline
	repoAdapter := &repositoryAdapter{repo: repo}

	// Build pipeline stages
	stages := []pipeline.Stage{
		pipeline.NewValidationStage(cache, creditService, logger),
		pipeline.NewRenderStage(templateSvc, logger),
		pipeline.NewSecurityStage(spamDetector, contentSanitizer, attachmentValidator, deliverabilityValidator, logger),
		pipeline.NewPersistenceStage(repoAdapter, cache, creditService, logger),
		pipeline.NewDispatchStage(queue, logger),
	}

	// Create pipeline
	emailPipeline := pipeline.NewEmailSendPipeline(stages)

	return &service{
		pipeline: emailPipeline,
		repo:     repo,
		logger:   logger,
	}
}

// repositoryAdapter adapts email.Repository to pipeline.EmailRepository.
type repositoryAdapter struct {
	repo Repository
}

func (a *repositoryAdapter) Create(ctx context.Context, model *pipeline.EmailModel) error {
	emailModel := &Model{
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

// Send sends an email using the pipeline architecture.
func (s *service) Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error) {
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

	// Create pipeline state
	state := &pipeline.State{
		UserID:         userID,
		From:           req.From,
		To:             req.To,
		Subject:        req.Subject,
		HTML:           req.HTML,
		Text:           req.Text,
		ReplyTo:        &req.ReplyTo,
		Headers:        req.Headers,
		TemplateID:     req.TemplateID,
		Variables:      req.Variables,
		Tags:           req.Tags,
		Attachments:    pipelineAttachments,
		IdempotencyKey: idempotencyKey,
	}

	// Execute pipeline
	result, err := s.pipeline.Execute(ctx, state)
	if err != nil {
		return nil, err
	}

	// Return response
	return &SendResponse{
		ID:        result.EmailID,
		MessageID: result.MessageID,
		Status:    result.Status,
	}, nil
}

func (s *service) GetLog(ctx context.Context, id uuid.UUID) (*Model, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) SearchLogs(ctx context.Context, q *LogQuery) ([]Model, int64, error) {
	return s.repo.Search(ctx, q)
}
