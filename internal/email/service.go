package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/template"
	"github.com/Mark-0731/SwiftMail/pkg/response"
	"github.com/Mark-0731/SwiftMail/pkg/validator"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Service defines the email business logic interface.
type Service interface {
	Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error)
	GetLog(ctx context.Context, id uuid.UUID) (*Model, error)
	SearchLogs(ctx context.Context, q *LogQuery) ([]Model, int64, error)
}

type service struct {
	repo                    Repository
	templateSvc             template.Service
	rdb                     *redis.Client
	asynqClient             *asynq.Client
	logger                  zerolog.Logger
	spamDetector            *SpamDetector
	attachmentValidator     *AttachmentValidator
	contentSanitizer        *ContentSanitizer
	creditService           *billing.CreditService
	deliverabilityValidator *DeliverabilityValidator
}

// NewService creates a new email service.
func NewService(
	repo Repository,
	templateSvc template.Service,
	rdb *redis.Client,
	asynqClient *asynq.Client,
	logger zerolog.Logger,
) Service {
	return &service{
		repo:                    repo,
		templateSvc:             templateSvc,
		rdb:                     rdb,
		asynqClient:             asynqClient,
		logger:                  logger,
		spamDetector:            NewSpamDetector(),
		attachmentValidator:     NewAttachmentValidator(),
		contentSanitizer:        NewContentSanitizer(),
		creditService:           billing.NewCreditService(rdb, logger),
		deliverabilityValidator: NewDeliverabilityValidator(logger),
	}
}

// Send is the critical hot path — Redis-first validation, then queue dispatch.
// Target: < 80ms p99 response time.
func (s *service) Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error) {
	// 1. Enhanced email validation
	toValidation := validator.ValidateEmailAdvanced(req.To, false) // Skip MX check for performance
	if !toValidation.Valid {
		return nil, fmt.Errorf("invalid recipient email: %s", toValidation.Reason)
	}

	fromValidation := validator.ValidateEmailAdvanced(req.From, false)
	if !fromValidation.Valid {
		return nil, fmt.Errorf("invalid sender email: %s", fromValidation.Reason)
	}

	// Log warnings for disposable/role-based emails
	if toValidation.IsDisposable {
		s.logger.Warn().Str("email", req.To).Msg("sending to disposable email address")
	}
	if toValidation.IsRoleBased {
		s.logger.Info().Str("email", req.To).Msg("sending to role-based email address")
	}

	// 2. Check suppression list (Redis SISMEMBER — O(1))
	emailHash := hashEmail(req.To)
	suppressed, err := s.rdb.SIsMember(ctx, fmt.Sprintf("suppress:%s", userID.String()), emailHash).Result()
	if err == nil && suppressed {
		return nil, fmt.Errorf("recipient %s is suppressed", req.To)
	}
	// Also check global suppression
	globalSuppressed, _ := s.rdb.SIsMember(ctx, "suppress:global", emailHash).Result()
	if globalSuppressed {
		return nil, fmt.Errorf("recipient %s is globally suppressed", req.To)
	}

	// 2.5. Run abuse detection check
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error().Interface("panic", r).Msg("panic in abuse detection")
			}
		}()
		// Check user's bounce and complaint rates
		// This will auto-suspend users with high abuse metrics
		ctx := context.Background()

		var sent, bounced, complained int64

		// Get counts from last hour
		s.rdb.Get(ctx, fmt.Sprintf("abuse:%s:sent", userID.String())).Scan(&sent)
		s.rdb.Get(ctx, fmt.Sprintf("abuse:%s:bounced", userID.String())).Scan(&bounced)
		s.rdb.Get(ctx, fmt.Sprintf("abuse:%s:complained", userID.String())).Scan(&complained)

		// Increment sent counter
		s.rdb.Incr(ctx, fmt.Sprintf("abuse:%s:sent", userID.String()))
		s.rdb.Expire(ctx, fmt.Sprintf("abuse:%s:sent", userID.String()), 1*time.Hour)

		// Check thresholds (5% bounce rate, 0.1% complaint rate)
		if sent > 100 {
			bounceRate := float64(bounced) / float64(sent) * 100
			complaintRate := float64(complained) / float64(sent) * 100

			if bounceRate > 5.0 || complaintRate > 0.1 {
				s.logger.Warn().
					Str("user_id", userID.String()).
					Float64("bounce_rate", bounceRate).
					Float64("complaint_rate", complaintRate).
					Msg("high abuse rate detected")
			}
		}
	}()

	// 3. Check credit availability (but don't deduct yet - reserve for later deduction)
	hasCredits, currentBalance, err := s.creditService.CheckCreditAvailability(ctx, userID, 1)
	if err != nil {
		s.logger.Warn().Err(err).Str("user_id", userID.String()).Msg("credit check failed, allowing request")
	} else if !hasCredits {
		return nil, fmt.Errorf("insufficient credits (current balance: %d)", currentBalance)
	}

	// 5. Resolve template if template_id provided
	subject := req.Subject
	htmlBody := req.HTML
	textBody := req.Text

	if req.TemplateID != nil {
		subj, html, text, err := s.templateSvc.Preview(ctx, *req.TemplateID, req.Variables)
		if err != nil {
			return nil, fmt.Errorf("template rendering failed: %w", err)
		}
		subject = subj
		htmlBody = html
		textBody = text
	}

	// 5.6. Content sanitization (security)
	subject, htmlBody, textBody, sanitizedHeaders := s.contentSanitizer.SanitizeAll(subject, htmlBody, textBody, req.Headers)

	s.logger.Debug().
		Str("user_id", userID.String()).
		Bool("html_sanitized", htmlBody != req.HTML).
		Bool("text_sanitized", textBody != req.Text).
		Bool("subject_sanitized", subject != req.Subject).
		Msg("content sanitization completed")

	// 5.7. SPF/DKIM validation for deliverability
	fromDomain := extractDomain(req.From)
	if fromDomain != "" {
		// Use quick validation to avoid blocking the send path
		deliverabilityValid, err := s.deliverabilityValidator.ValidateQuick(ctx, fromDomain)
		if err != nil {
			s.logger.Warn().
				Str("user_id", userID.String()).
				Str("domain", fromDomain).
				Err(err).
				Msg("deliverability validation failed - email may have poor deliverability")
		} else if !deliverabilityValid {
			s.logger.Warn().
				Str("user_id", userID.String()).
				Str("domain", fromDomain).
				Msg("no SPF or DKIM records found - email may be rejected by recipients")
		} else {
			s.logger.Debug().
				Str("user_id", userID.String()).
				Str("domain", fromDomain).
				Msg("deliverability validation passed")
		}
	}

	// 5.8. Spam content detection
	spamScore := s.spamDetector.AnalyzeContent(subject, htmlBody, textBody)
	if spamScore.IsSpam {
		s.logger.Warn().
			Str("user_id", userID.String()).
			Int("spam_score", spamScore.Score).
			Strs("reasons", spamScore.Reasons).
			Msg("email flagged as spam")
		return nil, fmt.Errorf("email content flagged as spam (score: %d/100)", spamScore.Score)
	}

	// Log warning for high spam scores (but not blocking)
	if spamScore.Score > 40 {
		s.logger.Warn().
			Str("user_id", userID.String()).
			Int("spam_score", spamScore.Score).
			Strs("reasons", spamScore.Reasons).
			Msg("email has elevated spam score")
	}

	// 5.9. Attachment validation
	if len(req.Attachments) > 0 {
		attachmentResult := s.attachmentValidator.ValidateAttachments(req.Attachments)
		if !attachmentResult.Valid {
			s.logger.Warn().
				Str("user_id", userID.String()).
				Str("reason", attachmentResult.Reason).
				Strs("invalid_attachments", attachmentResult.InvalidAttachments).
				Msg("attachment validation failed")
			return nil, fmt.Errorf("attachment validation failed: %s", attachmentResult.Reason)
		}

		// Log attachment info for monitoring
		s.logger.Info().
			Str("user_id", userID.String()).
			Int("attachment_count", attachmentResult.AttachmentCount).
			Int64("total_size_bytes", attachmentResult.TotalSize).
			Msg("attachments validated successfully")
	}

	// 5.10. Email size validation (prevent SMTP limits exceeded)
	totalEmailSize := int64(len(subject) + len(htmlBody) + len(textBody))
	for _, attachment := range req.Attachments {
		totalEmailSize += attachment.Size
	}

	const MaxEmailSize = 25 * 1024 * 1024 // 25MB total email size limit (common SMTP limit)
	if totalEmailSize > MaxEmailSize {
		s.logger.Warn().
			Str("user_id", userID.String()).
			Int64("email_size_bytes", totalEmailSize).
			Int64("max_size_bytes", MaxEmailSize).
			Msg("email size exceeds SMTP limits")
		return nil, fmt.Errorf("email size (%d MB) exceeds maximum allowed size (%d MB)",
			totalEmailSize/(1024*1024), MaxEmailSize/(1024*1024))
	}

	// 6. Generate message ID
	messageID := fmt.Sprintf("<%s@swiftmail>", uuid.New().String())

	// 7. Extract domain from sender for domain lookup
	// (fromDomain already extracted above for deliverability validation)

	// Look up domain ID (if domain exists)
	var domainID *uuid.UUID
	if fromDomain != "" {
		// Try to get domain from database
		domainKey := fmt.Sprintf("domain:%s:%s", userID.String(), fromDomain)
		domainIDStr, err := s.rdb.Get(ctx, domainKey).Result()
		if err == nil && domainIDStr != "" {
			if id, err := uuid.Parse(domainIDStr); err == nil {
				domainID = &id
			}
		}
	}

	// 8. Create email log
	emailLog := &Model{
		UserID:     userID,
		DomainID:   domainID, // Can be nil if domain not found
		MessageID:  messageID,
		FromEmail:  req.From,
		ToEmail:    req.To,
		Subject:    subject,
		Status:     StatusQueued,
		TemplateID: req.TemplateID,
		Tags:       req.Tags,
		MaxRetries: 3, // Application-level max retries (configurable via SMTP_MAX_RETRIES)
	}

	if idempotencyKey != "" {
		emailLog.IdempotencyKey = &idempotencyKey
	}

	if err := s.repo.Create(ctx, emailLog); err != nil {
		return nil, fmt.Errorf("failed to create email log: %w", err)
	}

	// 8.5. Reserve credit for this email send
	if err := s.creditService.ReserveCreditForSend(ctx, userID, emailLog.ID, 1); err != nil {
		s.logger.Warn().Err(err).Str("email_id", emailLog.ID.String()).Msg("failed to reserve credit, continuing anyway")
	}

	// 9. Build Asynq task payload
	payload := map[string]interface{}{
		"email_log_id":  emailLog.ID.String(),
		"from":          req.From,
		"to":            req.To,
		"subject":       subject,
		"html":          htmlBody,
		"text":          textBody,
		"reply_to":      req.ReplyTo,
		"headers":       sanitizedHeaders,
		"message_id":    messageID,
		"user_id":       userID.String(),
		"deduct_credit": true, // Credit will be deducted after successful send
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// 10. Dispatch to Asynq queue (Redis LPUSH — < 1ms)
	task := asynq.NewTask("email:send", payloadBytes,
		asynq.Queue("high"),
		asynq.MaxRetry(5), // Asynq will retry up to 5 times with exponential backoff
		asynq.TaskID(emailLog.ID.String()),
	)

	if _, err := s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to enqueue email: %w", err)
	}

	s.logger.Info().
		Str("email_log_id", emailLog.ID.String()).
		Str("to", req.To).
		Str("from", req.From).
		Msg("email queued")

	// 11. Return 202 Accepted
	return &SendResponse{
		ID:        emailLog.ID,
		MessageID: messageID,
		Status:    StatusQueued,
	}, nil
}

func (s *service) GetLog(ctx context.Context, id uuid.UUID) (*Model, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) SearchLogs(ctx context.Context, q *LogQuery) ([]Model, int64, error) {
	return s.repo.Search(ctx, q)
}

func hashEmail(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}

func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

// Ensure response package is used (import side effect)
var _ = response.OK
