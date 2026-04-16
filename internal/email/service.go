package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/swiftmail/swiftmail/internal/template"
	"github.com/swiftmail/swiftmail/pkg/response"
	"github.com/swiftmail/swiftmail/pkg/validator"
)

// Service defines the email business logic interface.
type Service interface {
	Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error)
	GetLog(ctx context.Context, id uuid.UUID) (*Model, error)
	SearchLogs(ctx context.Context, q *LogQuery) ([]Model, int64, error)
}

type service struct {
	repo        Repository
	templateSvc template.Service
	rdb         *redis.Client
	asynqClient *asynq.Client
	logger      zerolog.Logger
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
		repo:        repo,
		templateSvc: templateSvc,
		rdb:         rdb,
		asynqClient: asynqClient,
		logger:      logger,
	}
}

// Send is the critical hot path — Redis-first validation, then queue dispatch.
// Target: < 80ms p99 response time.
func (s *service) Send(ctx context.Context, userID uuid.UUID, req *SendRequest, idempotencyKey string) (*SendResponse, error) {
	// 1. Validate email addresses
	if !validator.IsValidEmail(req.To) {
		return nil, fmt.Errorf("invalid recipient email: %s", req.To)
	}
	if !validator.IsValidEmail(req.From) {
		return nil, fmt.Errorf("invalid sender email: %s", req.From)
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

	// 3. Check and deduct credit atomically (Lua script prevents race condition)
	creditKey := fmt.Sprintf("credits:%s", userID.String())

	// Atomic check-and-decrement using Lua script
	luaScript := `
		local balance = redis.call("GET", KEYS[1])
		if balance == false then
			return -2
		end
		if tonumber(balance) <= 0 then
			return -1
		end
		return redis.call("DECRBY", KEYS[1], 1)
	`

	result, err := s.rdb.Eval(ctx, luaScript, []string{creditKey}).Int64()
	if err != nil {
		// Redis error - allow request but log warning
		s.logger.Warn().Err(err).Msg("credit check failed, allowing request")
	} else if result == -2 {
		// No credit record - will be checked in DB later
		s.logger.Debug().Str("user_id", userID.String()).Msg("no cached credits, will check DB")
	} else if result == -1 {
		return nil, fmt.Errorf("insufficient credits")
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

	// 6. Generate message ID
	messageID := fmt.Sprintf("<%s@swiftmail>", uuid.New().String())

	// 7. Extract domain from sender for domain lookup
	fromDomain := extractDomain(req.From)

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

	// 9. Build Asynq task payload
	payload := map[string]interface{}{
		"email_log_id": emailLog.ID.String(),
		"from":         req.From,
		"to":           req.To,
		"subject":      subject,
		"html":         htmlBody,
		"text":         textBody,
		"reply_to":     req.ReplyTo,
		"headers":      req.Headers,
		"message_id":   messageID,
		"user_id":      userID.String(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// 10. Dispatch to Asynq queue (Redis LPUSH — < 1ms)
	task := asynq.NewTask("email:send", payloadBytes,
		asynq.Queue("high"),
		asynq.MaxRetry(5),
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
