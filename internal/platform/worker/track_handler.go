package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/features/analytics"
	analyticsapp "github.com/Mark-0731/SwiftMail/internal/features/analytics/application"
	emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	emailinfra "github.com/Mark-0731/SwiftMail/internal/features/email/infrastructure"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// TrackHandler processes tracking events (open/click).
type TrackHandler struct {
	emailRepo        emailinfra.EmailRepository
	analyticsService *analyticsapp.Service
	logger           zerolog.Logger
}

// NewTrackHandler creates a new tracking handler.
func NewTrackHandler(emailRepo emailinfra.EmailRepository, analyticsService *analyticsapp.Service, logger zerolog.Logger) *TrackHandler {
	return &TrackHandler{
		emailRepo:        emailRepo,
		analyticsService: analyticsService,
		logger:           logger,
	}
}

func (h *TrackHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload TrackingEventPayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal tracking payload: %w", err)
	}

	// Get email details for analytics
	emailLog, err := h.emailRepo.GetByID(ctx, payload.EmailLogID)
	if err != nil {
		h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to get email log")
		// Continue anyway to update tracking
	}

	switch payload.EventType {
	case "opened":
		if err := h.emailRepo.SetOpened(ctx, payload.EmailLogID); err != nil {
			h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to set opened")
			return err
		}

		// Track "opened" event in analytics
		if h.analyticsService != nil && emailLog != nil {
			h.analyticsService.TrackEvent(analytics.Event{
				UserID:    emailLog.UserID,
				EmailID:   payload.EmailLogID,
				EventType: "opened",
				Recipient: emailLog.ToEmail,
				IPAddress: payload.IPAddress,
				UserAgent: payload.UserAgent,
				Timestamp: time.Now().UTC(),
			})
			h.logger.Debug().Msg("tracked opened event")
		}

	case "clicked":
		if err := h.emailRepo.SetClicked(ctx, payload.EmailLogID); err != nil {
			h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to set clicked")
			return err
		}

		// Track "clicked" event in analytics
		if h.analyticsService != nil && emailLog != nil {
			h.analyticsService.TrackEvent(analytics.Event{
				UserID:    emailLog.UserID,
				EmailID:   payload.EmailLogID,
				EventType: "clicked",
				Recipient: emailLog.ToEmail,
				IPAddress: payload.IPAddress,
				UserAgent: payload.UserAgent,
				Timestamp: time.Now().UTC(),
			})
			h.logger.Debug().Msg("tracked clicked event")
		}
	}

	h.logger.Debug().
		Str("email_log_id", payload.EmailLogID.String()).
		Str("event", payload.EventType).
		Msg("tracking event processed")

	return nil
}

// BounceHandler processes bounce notifications.
type BounceHandler struct {
	emailRepo        emailinfra.EmailRepository
	analyticsService *analyticsapp.Service
	logger           zerolog.Logger
}

// NewBounceHandler creates a new bounce handler.
func NewBounceHandler(emailRepo emailinfra.EmailRepository, analyticsService *analyticsapp.Service, logger zerolog.Logger) *BounceHandler {
	return &BounceHandler{
		emailRepo:        emailRepo,
		analyticsService: analyticsService,
		logger:           logger,
	}
}

func (h *BounceHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload BouncePayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal bounce payload: %w", err)
	}

	status := emailtypes.StatusBounced
	eventType := "bounced"
	if payload.BounceType == "complaint" {
		status = emailtypes.StatusComplained
		eventType = "complained"
	}

	resp := payload.Diagnostic
	if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, emailtypes.StatusSent, status, &resp); err != nil {
		h.logger.Error().Err(err).Msg("failed to update bounce status")
		return err
	}

	// Get email details for analytics
	emailLog, err := h.emailRepo.GetByID(ctx, payload.EmailLogID)
	if err != nil {
		h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to get email log")
		// Continue anyway
	}

	// Track bounce/complaint event in analytics
	if h.analyticsService != nil && emailLog != nil {
		h.analyticsService.TrackEvent(analytics.Event{
			UserID:    emailLog.UserID,
			EmailID:   payload.EmailLogID,
			EventType: eventType,
			Recipient: payload.Recipient,
			Timestamp: time.Now().UTC(),
		})
		h.logger.Debug().Str("event_type", eventType).Msg("tracked bounce/complaint event")
	}

	h.logger.Info().
		Str("email_log_id", payload.EmailLogID.String()).
		Str("bounce_type", payload.BounceType).
		Str("recipient", payload.Recipient).
		Msg("bounce processed")

	return nil
}
