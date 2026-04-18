package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/Mark-0731/SwiftMail/internal/email"
	smtpengine "github.com/Mark-0731/SwiftMail/internal/smtp"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
)

// SendHandler processes email:send tasks.
type SendHandler struct {
	emailRepo  email.Repository
	smtpSender *smtpengine.Sender
	metrics    *metrics.Metrics
	logger     zerolog.Logger
}

// NewSendHandler creates a new send handler.
func NewSendHandler(
	emailRepo email.Repository,
	smtpSender *smtpengine.Sender,
	m *metrics.Metrics,
	logger zerolog.Logger,
) *SendHandler {
	return &SendHandler{
		emailRepo:  emailRepo,
		smtpSender: smtpSender,
		metrics:    m,
		logger:     logger,
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
	if currentStatus == email.StatusQueued || currentStatus == email.StatusDeferred {
		if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, email.StatusProcessing, nil); err != nil {
			log.Warn().Err(err).Msg("failed to update to processing state")
		}
		currentStatus = email.StatusProcessing
	}

	// Send via SMTP engine
	smtpResponse, err := h.smtpSender.Send(ctx, &smtpengine.SendRequest{
		From:      payload.From,
		To:        payload.To,
		Subject:   payload.Subject,
		HTML:      payload.HTML,
		Text:      payload.Text,
		ReplyTo:   payload.ReplyTo,
		Headers:   payload.Headers,
		MessageID: payload.MessageID,
	})

	duration := time.Since(start).Seconds()
	h.metrics.EmailDeliveryDuration.Observe(duration)

	if err != nil {
		log.Error().Err(err).Str("smtp_response", smtpResponse).Msg("SMTP send failed")

		// Classify error type
		isTemporary := isTemporaryError(smtpResponse)

		if isTemporary {
			// Temporary error (4xx) - let Asynq retry
			h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, email.StatusDeferred, &smtpResponse)
			h.metrics.EmailsSentTotal.WithLabelValues("deferred", extractDomain(payload.To)).Inc()
			log.Info().Msg("temporary SMTP error - Asynq will retry")

			// Return error to trigger Asynq retry
			return fmt.Errorf("temporary SMTP error: %s", smtpResponse)
		}

		// Permanent error (5xx) - don't retry
		h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, email.StatusFailed, &smtpResponse)
		h.metrics.EmailsSentTotal.WithLabelValues("failed", extractDomain(payload.To)).Inc()
		log.Info().Msg("permanent SMTP error - not retrying")

		// Return nil to prevent Asynq retry
		return nil
	}

	// Success: processing → sent
	h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, currentStatus, email.StatusSent, &smtpResponse)
	h.metrics.EmailsSentTotal.WithLabelValues("sent", extractDomain(payload.To)).Inc()

	log.Info().
		Float64("duration_ms", duration*1000).
		Str("smtp_response", smtpResponse).
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
