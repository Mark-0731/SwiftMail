package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/Mark-0731/SwiftMail/internal/email"
)

// TrackHandler processes tracking events (open/click).
type TrackHandler struct {
	emailRepo email.Repository
	logger    zerolog.Logger
}

// NewTrackHandler creates a new tracking handler.
func NewTrackHandler(emailRepo email.Repository, logger zerolog.Logger) *TrackHandler {
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
	emailRepo email.Repository
	logger    zerolog.Logger
}

// NewBounceHandler creates a new bounce handler.
func NewBounceHandler(emailRepo email.Repository, logger zerolog.Logger) *BounceHandler {
	return &BounceHandler{emailRepo: emailRepo, logger: logger}
}

func (h *BounceHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload BouncePayload
	if err := UnmarshalPayload(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal bounce payload: %w", err)
	}

	status := email.StatusBounced
	if payload.BounceType == "complaint" {
		status = email.StatusComplained
	}

	resp := payload.Diagnostic
	if err := h.emailRepo.UpdateStatus(ctx, payload.EmailLogID, email.StatusSent, status, &resp); err != nil {
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
