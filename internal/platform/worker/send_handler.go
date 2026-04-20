package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/config"
	emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	emailinfra "github.com/Mark-0731/SwiftMail/internal/features/email/infrastructure"
	"github.com/Mark-0731/SwiftMail/internal/platform/provider"
	"github.com/Mark-0731/SwiftMail/internal/shared/events"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
	"github.com/Mark-0731/SwiftMail/pkg/tracking"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// SendHandler processes email:send tasks.
type SendHandler struct {
	emailRepo emailinfra.EmailRepository
	provider  provider.Provider
	eventBus  events.Bus
	metrics   *metrics.Metrics
	logger    zerolog.Logger
	config    *config.Config
}

// NewSendHandler creates a new send handler.
func NewSendHandler(
	emailRepo emailinfra.EmailRepository,
	provider provider.Provider,
	eventBus events.Bus,
	m *metrics.Metrics,
	cfg *config.Config,
	logger zerolog.Logger,
) *SendHandler {
	return &SendHandler{
		emailRepo: emailRepo,
		provider:  provider,
		eventBus:  eventBus,
		metrics:   m,
		config:    cfg,
		logger:    logger,
	}
}

// ProcessTask handles the email:send task.
// DESIGN: Asynq handles retries automatically - we just return errors
func (h *SendHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	start := time.Now()
	var payload EmailSendPayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	log := h.logger.With().
		Str("email_log_id", payload.EmailLogID.String()).
		Str("to", payload.To).
		Str("from", payload.From).
		Logger()
	// Get current email status for proper state transitions
	emailLog, err := h.emailRepo.GetByID(ctx, payload.EmailLogID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get email log")
		return fmt.Errorf("failed to get email log: %w", err)
	}
	currentStatus := emailLog.Status
	// Update to processing state (if not already)
	if currentStatus == emailtypes.StatusQueued || currentStatus == emailtypes.StatusDeferred {
		if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusProcessing, nil); err != nil {
			log.Warn().Err(err).Msg("failed to update to processing state")
		}
		currentStatus = emailtypes.StatusProcessing
	}
	// Apply click tracking and open tracking to HTML content
	htmlContent := payload.HTML
	if htmlContent != "" && h.config.App.BaseURL != "" {
		// Rewrite links for click tracking
		htmlContent = tracking.RewriteLinks(htmlContent, payload.EmailLogID.String(), h.config.App.BaseURL)
		// Inject open tracking pixel
		htmlContent = tracking.InjectPixel(htmlContent, payload.EmailLogID.String(), h.config.App.BaseURL)
	}
	// Send via provider (SMTP or SendGrid)
	var replyTo *string
	if payload.ReplyTo != "" {
		replyTo = &payload.ReplyTo
	}
	providerResp, err := h.provider.Send(ctx, &provider.SendRequest{
		From:      payload.From,
		To:        payload.To,
		Subject:   payload.Subject,
		HTML:      htmlContent, // Use modified HTML with tracking
		Text:      payload.Text,
		ReplyTo:   replyTo,
		Headers:   payload.Headers,
		MessageID: payload.MessageID,
	})
	duration := time.Since(start).Seconds()
	h.metrics.EmailDeliveryDuration.Observe(duration)
	if err != nil {
		errorMsg := err.Error()
		if providerResp != nil && providerResp.Error != "" {
			errorMsg = providerResp.Error
		}
		log.Error().Err(err).Str("provider_response", errorMsg).Msg("provider send failed")
		// Classify error type
		isTemporary := isTemporaryError(errorMsg)
		if isTemporary {
			// Temporary error (4xx) - let Asynq retry
			h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusDeferred, &errorMsg)
			h.metrics.EmailsSentTotal.WithLabelValues("deferred", extractDomain(payload.To)).Inc()
			log.Info().Msg("temporary error - Asynq will retry")
			// Return error to trigger Asynq retry
			return fmt.Errorf("temporary error: %s", errorMsg)
		}
		// Permanent error (5xx) - don't retry
		h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusFailed, &errorMsg)
		h.metrics.EmailsSentTotal.WithLabelValues("failed", extractDomain(payload.To)).Inc()
		log.Info().Msg("permanent error - not retrying")
		// Publish email.failed event
		event := events.EmailFailedEvent(payload.EmailLogID, payload.UserID, errorMsg)
		if err := h.eventBus.Publish(ctx, event); err != nil {
			log.Warn().Err(err).Msg("failed to publish email.failed event")
		}
		// Return nil to prevent Asynq retry
		return nil
	}
	// Success: processing → sent
	successMsg := providerResp.ProviderMessageID
	h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, emailtypes.StatusSent, &successMsg)
	h.metrics.EmailsSentTotal.WithLabelValues("sent", extractDomain(payload.To)).Inc()
	// Publish email.sent event
	event := events.EmailSentEvent(payload.EmailLogID, payload.UserID, payload.To)
	if err := h.eventBus.Publish(ctx, event); err != nil {
		log.Warn().Err(err).Msg("failed to publish email.sent event")
	}
	log.Info().
		Float64("duration_ms", duration*1000).
		Str("provider_message_id", providerResp.ProviderMessageID).
		Msg("email sent successfully")
	return nil
}
func isTemporaryError(response string) bool {
	if len(response) == 0 {
		return true // Assume temporary if no response
	}
	// 4xx codes are temporary
	if response[0] == '4' {
		return true
	}
	return false
}
func extractDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return ""
}
