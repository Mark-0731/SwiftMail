package worker

import (
	"context"
	"fmt"

	emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	emailinfra "github.com/Mark-0731/SwiftMail/internal/features/email/infrastructure"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// TrackHandler processes tracking events (open/click).
type TrackHandler struct {
	emailRepo emailinfra.EmailRepository
	logger    zerolog.Logger
}

// NewTrackHandler creates a new tracking handler.
func NewTrackHandler(emailRepo emailinfra.EmailRepository, logger zerolog.Logger) *TrackHandler {
	return &TrackHandler{emailRepo: emailRepo, logger: logger}
}
func (h *TrackHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload TrackingEventPayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal tracking payload: %w", err)
	}
	switch payload.EventType {
	case "opened":
		if err := h.emailRepo.SetOpened(ctx, payload.EmailLogID); err != nil {
			h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to set opened")
			return err
		}
	case "clicked":
		if err := h.emailRepo.SetClicked(ctx, payload.EmailLogID); err != nil {
			h.logger.Error().Err(err).Str("email_log_id", payload.EmailLogID.String()).Msg("failed to set clicked")
			return err
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
	emailRepo emailinfra.EmailRepository
	logger    zerolog.Logger
}

// NewBounceHandler creates a new bounce handler.
func NewBounceHandler(emailRepo emailinfra.EmailRepository, logger zerolog.Logger) *BounceHandler {
	return &BounceHandler{emailRepo: emailRepo, logger: logger}
}
func (h *BounceHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload BouncePayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal bounce payload: %w", err)
	}
	status := emailtypes.StatusBounced
	if payload.BounceType == "complaint" {
		status = emailtypes.StatusComplained
	}
	resp := payload.Diagnostic
	if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, emailtypes.StatusSent, status, &resp); err != nil {
		h.logger.Error().Err(err).Msg("failed to update bounce status")
		return err
	}
	h.logger.Info().
		Str("email_log_id", payload.EmailLogID.String()).
		Str("bounce_type", payload.BounceType).
		Str("recipient", payload.Recipient).
		Msg("bounce processed")
	return nil
}
