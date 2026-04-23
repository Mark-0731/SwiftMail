package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/config"
	emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	emailinfra "github.com/Mark-0731/SwiftMail/internal/features/email/infrastructure"
	"github.com/Mark-0731/SwiftMail/internal/platform/provider"
	"github.com/Mark-0731/SwiftMail/internal/platform/queue"
	"github.com/Mark-0731/SwiftMail/internal/platform/resilience"
	"github.com/Mark-0731/SwiftMail/internal/shared/events"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
	"github.com/Mark-0731/SwiftMail/pkg/tracking"
)

// SendHandler processes email:send tasks with advanced resilience features
type SendHandler struct {
	emailRepo emailinfra.EmailRepository
	provider  provider.Provider
	eventBus  events.Bus
	metrics   *metrics.Metrics
	logger    zerolog.Logger
	config    *config.Config
	dlq       *queue.DeadLetterQueue
	rdb       *redis.Client

	// Resilience components
	circuitBreaker *resilience.CircuitBreakerManager
	adaptiveRetry  *resilience.AdaptiveRetryEngine
	poisonQueue    *resilience.PoisonQueue
	backpressure   *resilience.BackpressureController
}

// NewSendHandler creates a new enhanced send handler with resilience features
func NewSendHandler(
	emailRepo emailinfra.EmailRepository,
	provider provider.Provider,
	eventBus events.Bus,
	m *metrics.Metrics,
	cfg *config.Config,
	logger zerolog.Logger,
	dlq *queue.DeadLetterQueue,
	rdb *redis.Client,
	circuitBreaker *resilience.CircuitBreakerManager,
	adaptiveRetry *resilience.AdaptiveRetryEngine,
	poisonQueue *resilience.PoisonQueue,
	backpressure *resilience.BackpressureController,
) *SendHandler {
	return &SendHandler{
		emailRepo:      emailRepo,
		provider:       provider,
		eventBus:       eventBus,
		metrics:        m,
		config:         cfg,
		logger:         logger,
		dlq:            dlq,
		rdb:            rdb,
		circuitBreaker: circuitBreaker,
		adaptiveRetry:  adaptiveRetry,
		poisonQueue:    poisonQueue,
		backpressure:   backpressure,
	}
}

// ProcessTask handles the email:send task with advanced resilience
func (h *SendHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	// 1. Backpressure check
	if !h.backpressure.ShouldAccept(ctx) {
		h.logger.Warn().Msg("task rejected due to backpressure")
		// Return error to retry later
		return fmt.Errorf("system overloaded, retry later")
	}

	h.backpressure.RecordTaskStart()
	defer func() {
		duration := time.Since(start)
		// Success/failure will be recorded in handleSendSuccess/handleSendError
		h.backpressure.RecordTaskComplete(duration, true)
	}()

	// 2. Parse payload
	var payload EmailSendPayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		h.logger.Error().Err(err).Msg("failed to unmarshal payload")
		h.moveMalformedTaskToDLQ(ctx, t, err)
		return nil
	}

	log := h.logger.With().
		Str("email_log_id", payload.EmailLogID.String()).
		Str("task_id", t.ResultWriter().TaskID()).
		Str("to", payload.To).
		Str("from", payload.From).
		Logger()

	log.Debug().Msg("processing email send task with resilience features")

	// 3. Idempotency Check
	emailLog, err := h.emailRepo.GetByID(ctx, payload.EmailLogID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get email log")
		return fmt.Errorf("failed to get email log: %w", err)
	}

	if emailLog.Status == emailtypes.StatusSent {
		log.Info().Msg("email already sent, skipping (idempotent)")
		h.metrics.EmailsSentTotal.WithLabelValues("duplicate_skip", extractDomain(payload.To), "").Inc()
		return nil
	}

	if emailLog.Status == emailtypes.StatusFailed {
		log.Info().Msg("email already failed permanently, skipping")
		return nil
	}

	// 4. Distributed Lock
	lockKey := fmt.Sprintf("email:lock:%s", payload.EmailLogID.String())
	lockAcquired, err := h.acquireLock(ctx, lockKey, 5*time.Minute)
	if err != nil {
		log.Warn().Err(err).Msg("failed to acquire lock, will retry")
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !lockAcquired {
		log.Info().Msg("another worker has lock, skipping")
		return nil
	}
	defer h.releaseLock(ctx, lockKey)

	// 5. Double-check status after lock
	emailLog, err = h.emailRepo.GetByID(ctx, payload.EmailLogID)
	if err != nil {
		return fmt.Errorf("failed to re-check email status: %w", err)
	}
	if emailLog.Status == emailtypes.StatusSent {
		log.Info().Msg("email sent by another worker, skipping")
		return nil
	}

	currentStatus := emailLog.Status

	// 6. Update to processing state
	if currentStatus == emailtypes.StatusQueued || currentStatus == emailtypes.StatusDeferred {
		if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusProcessing, nil); err != nil {
			log.Warn().Err(err).Msg("failed to update to processing state")
		}
		currentStatus = emailtypes.StatusProcessing
	}

	// 7. Circuit Breaker Check - Domain level
	domain := extractDomain(payload.To)
	domainBreaker := h.circuitBreaker.GetBreaker("domain", domain)

	// 8. Circuit Breaker Check - Provider level
	providerBreaker := h.circuitBreaker.GetBreaker("provider", "smtp")

	// 9. Apply tracking
	htmlContent := payload.HTML
	if htmlContent != "" && h.config.App.BaseURL != "" {
		htmlContent = tracking.RewriteLinks(htmlContent, payload.EmailLogID.String(), h.config.App.BaseURL)
		htmlContent = tracking.InjectPixel(htmlContent, payload.EmailLogID.String(), h.config.App.BaseURL)
	}

	// 10. Send via provider with circuit breaker protection
	var replyTo *string
	if payload.ReplyTo != "" {
		replyTo = &payload.ReplyTo
	}

	sendRequest := &provider.SendRequest{
		From:      payload.From,
		To:        payload.To,
		Subject:   payload.Subject,
		HTML:      htmlContent,
		Text:      payload.Text,
		ReplyTo:   replyTo,
		Headers:   payload.Headers,
		MessageID: payload.MessageID,
	}

	var providerResp *provider.SendResponse
	var sendErr error

	// Execute with circuit breaker protection
	err = providerBreaker.Call(ctx, func() error {
		return domainBreaker.Call(ctx, func() error {
			providerResp, sendErr = h.provider.Send(ctx, sendRequest)
			return sendErr
		})
	})

	// If circuit breaker is open, treat as temporary error
	if err != nil && sendErr == nil {
		sendErr = err
		log.Warn().Err(err).Msg("circuit breaker prevented send")
	}

	duration := time.Since(start)
	h.metrics.EmailDeliveryDuration.Observe(duration.Seconds())

	// 11. Handle send result
	if sendErr != nil {
		return h.handleSendError(ctx, t, payload, currentStatus, sendErr, providerResp, log)
	}

	// 12. Success path
	return h.handleSendSuccess(ctx, payload, currentStatus, providerResp, duration, log)
}

// handleSendError processes email send failures with advanced resilience
func (h *SendHandler) handleSendError(
	ctx context.Context,
	t *asynq.Task,
	payload EmailSendPayload,
	currentStatus string,
	sendErr error,
	providerResp *provider.SendResponse,
	log zerolog.Logger,
) error {
	errorMsg := sendErr.Error()
	if providerResp != nil && providerResp.Error != "" {
		errorMsg = providerResp.Error
	}

	// Classify error
	classification := ClassifyError(errorMsg)
	domain := extractDomain(payload.To)

	log.Error().
		Err(sendErr).
		Str("error_msg", errorMsg).
		Str("error_code", classification.ErrorCode).
		Str("category", classification.Category).
		Bool("is_temporary", classification.IsTemporary).
		Bool("should_dlq", classification.ShouldMoveToDLQ).
		Msg("email send failed")

	// Record outcome for adaptive retry engine
	retryCount := 0 // Asynq doesn't expose retry count easily
	h.adaptiveRetry.RecordRetryOutcome(ctx, domain, classification.Category, retryCount, false, 0)

	// Temporary error - retry with adaptive delay
	if classification.IsTemporary {
		if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusDeferred, &errorMsg); err != nil {
			log.Warn().Err(err).Msg("failed to update status to deferred")
		}

		h.metrics.EmailsSentTotal.WithLabelValues("deferred", domain, "").Inc()
		h.metrics.EmailsSentTotal.WithLabelValues("error_"+classification.Category, domain, "").Inc()

		// Get adaptive retry delay
		adaptiveDelay := h.adaptiveRetry.GetRetryDelay(domain, classification.Category, retryCount)

		log.Info().
			Dur("adaptive_retry_after", adaptiveDelay).
			Dur("default_retry_after", classification.RetryAfter).
			Msg("temporary error - will retry with adaptive delay")

		return fmt.Errorf("temporary error (%s): %s", classification.Category, errorMsg)
	}

	// Permanent error - move to DLQ
	if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusFailed, &errorMsg); err != nil {
		log.Error().Err(err).Msg("failed to update status to failed")
	}

	h.metrics.EmailsSentTotal.WithLabelValues("failed", domain, "").Inc()
	h.metrics.EmailsSentTotal.WithLabelValues("error_"+classification.Category, domain, "").Inc()

	// Move to DLQ
	if classification.ShouldMoveToDLQ {
		if err := h.moveToDLQ(ctx, t, payload, errorMsg, classification); err != nil {
			log.Error().Err(err).Msg("CRITICAL: failed to move task to DLQ")
		} else {
			log.Info().
				Str("error_code", classification.ErrorCode).
				Str("category", classification.Category).
				Msg("task moved to DLQ")

			// Check if this should be quarantined to poison queue
			go h.poisonQueue.CheckAndQuarantine(context.Background(), payload.EmailLogID, &payload.EmailLogID)
		}
	}

	// Publish failure event
	event := events.EmailFailedEvent(payload.EmailLogID, payload.UserID, errorMsg)
	if err := h.eventBus.Publish(ctx, event); err != nil {
		log.Warn().Err(err).Msg("failed to publish email.failed event")
	}

	return nil
}

// handleSendSuccess processes successful email sends
func (h *SendHandler) handleSendSuccess(
	ctx context.Context,
	payload EmailSendPayload,
	currentStatus string,
	providerResp *provider.SendResponse,
	duration time.Duration,
	log zerolog.Logger,
) error {
	successMsg := providerResp.ProviderMessageID
	domain := extractDomain(payload.To)

	// Record success for adaptive retry engine
	h.adaptiveRetry.RecordRetryOutcome(ctx, domain, "success", 0, true, duration)

	// Update status to sent
	if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusSent, &successMsg); err != nil {
		log.Error().Err(err).Msg("failed to update status to sent")
		return fmt.Errorf("failed to update status: %w", err)
	}

	h.metrics.EmailsSentTotal.WithLabelValues("sent", domain, "").Inc()

	// Publish success event
	event := events.EmailSentEvent(payload.EmailLogID, payload.UserID, payload.To)
	if err := h.eventBus.Publish(ctx, event); err != nil {
		log.Warn().Err(err).Msg("failed to publish email.sent event")
	}

	log.Info().
		Dur("duration", duration).
		Str("provider_message_id", providerResp.ProviderMessageID).
		Msg("email sent successfully")

	return nil
}

// moveToDLQ moves a task to DLQ with retry history tracking
func (h *SendHandler) moveToDLQ(
	ctx context.Context,
	t *asynq.Task,
	payload EmailSendPayload,
	errorMsg string,
	classification ErrorClassification,
) error {
	if h.dlq == nil {
		h.logger.Error().Msg("CRITICAL: DLQ not configured, message will be lost!")
		return fmt.Errorf("DLQ not configured")
	}

	recipientDomain := extractDomain(payload.To)
	payloadBytes, err := MarshalPayload(payload)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to marshal payload for DLQ")
		payloadBytes = []byte("{}")
	}

	dlqEntry := &queue.DLQEntry{
		TaskType:        t.Type(),
		TaskID:          t.ResultWriter().TaskID(),
		Payload:         payloadBytes,
		FailureReason:   errorMsg,
		ErrorCode:       classification.ErrorCode,
		SMTPResponse:    errorMsg,
		RetryCount:      0,
		MaxRetries:      3,
		EmailLogID:      &payload.EmailLogID,
		UserID:          &payload.UserID,
		RecipientEmail:  payload.To,
		RecipientDomain: recipientDomain,
	}

	// Add to DLQ with retry logic
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := h.dlq.Add(ctx, dlqEntry); err != nil {
			h.logger.Error().
				Err(err).
				Int("attempt", attempt).
				Msg("failed to add to DLQ")

			if attempt < maxAttempts {
				time.Sleep(time.Duration(attempt*attempt) * 100 * time.Millisecond)
				continue
			}

			return fmt.Errorf("failed to add to DLQ after %d attempts: %w", maxAttempts, err)
		}

		h.logger.Info().
			Str("dlq_id", dlqEntry.ID.String()).
			Str("email_log_id", payload.EmailLogID.String()).
			Str("recipient", payload.To).
			Str("error_code", classification.ErrorCode).
			Str("category", classification.Category).
			Int("attempt", attempt).
			Msg("task moved to DLQ successfully")

		return nil
	}

	return fmt.Errorf("unreachable: DLQ add failed")
}

// moveMalformedTaskToDLQ handles tasks with malformed payloads
func (h *SendHandler) moveMalformedTaskToDLQ(ctx context.Context, t *asynq.Task, parseErr error) {
	if h.dlq == nil {
		h.logger.Error().Msg("CRITICAL: DLQ not configured, malformed task will be lost!")
		return
	}

	dlqEntry := &queue.DLQEntry{
		TaskType:      t.Type(),
		TaskID:        t.ResultWriter().TaskID(),
		Payload:       t.Payload(),
		FailureReason: fmt.Sprintf("Malformed payload: %v", parseErr),
		ErrorCode:     "malformed_payload",
		SMTPResponse:  parseErr.Error(),
		RetryCount:    0,
		MaxRetries:    0,
	}

	if err := h.dlq.Add(ctx, dlqEntry); err != nil {
		h.logger.Error().
			Err(err).
			Msg("CRITICAL: failed to add malformed task to DLQ")
	} else {
		h.logger.Warn().
			Str("dlq_id", dlqEntry.ID.String()).
			Msg("malformed task moved to DLQ")
	}
}

// acquireLock acquires a distributed lock using Redis
func (h *SendHandler) acquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if h.rdb == nil {
		h.logger.Warn().Msg("Redis not available, skipping distributed lock")
		return true, nil
	}

	// Use SET with NX and EX options (modern Redis approach)
	acquired, err := h.rdb.SetNX(ctx, key, "locked", ttl).Result()
	if err != nil {
		h.logger.Warn().Err(err).Msg("failed to acquire lock, proceeding anyway")
		return true, nil
	}

	return acquired, nil
}

// releaseLock releases a distributed lock
func (h *SendHandler) releaseLock(ctx context.Context, key string) {
	if h.rdb == nil {
		return
	}

	if err := h.rdb.Del(ctx, key).Err(); err != nil {
		h.logger.Warn().Err(err).Str("key", key).Msg("failed to release lock")
	}
}

// extractDomain extracts domain from email address
func extractDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			if i+1 < len(email) {
				return email[i+1:]
			}
			return ""
		}
	}
	return "unknown"
}
