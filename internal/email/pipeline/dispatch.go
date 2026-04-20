package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Mark-0731/SwiftMail/internal/infrastructure/queue"
	"github.com/rs/zerolog"
)

// DispatchStage handles queue dispatch.
type DispatchStage struct {
	queue  queue.Queue
	logger zerolog.Logger
}

// NewDispatchStage creates a new dispatch stage.
func NewDispatchStage(queue queue.Queue, logger zerolog.Logger) Stage {
	return &DispatchStage{
		queue:  queue,
		logger: logger,
	}
}

// Name returns the stage name.
func (s *DispatchStage) Name() string {
	return "dispatch"
}

// Execute dispatches the email to the queue.
func (s *DispatchStage) Execute(ctx context.Context, state *State) error {
	// Build task payload
	payload := map[string]interface{}{
		"email_log_id":  state.EmailLogID.String(),
		"from":          state.From,
		"to":            state.To,
		"subject":       state.RenderedSubject,
		"html":          state.RenderedHTML,
		"text":          state.RenderedText,
		"reply_to":      state.ReplyTo,
		"headers":       state.SanitizedHeaders,
		"message_id":    state.MessageID,
		"user_id":       state.UserID.String(),
		"deduct_credit": true,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// Enqueue task
	task := &queue.Task{
		Type:    "email:send",
		Payload: payloadBytes,
	}

	opts := &queue.EnqueueOptions{
		Queue:    "high",
		MaxRetry: 5,
		TaskID:   state.EmailLogID.String(),
	}

	if err := s.queue.EnqueueWithOptions(ctx, task, opts); err != nil {
		return fmt.Errorf("failed to enqueue email: %w", err)
	}

	s.logger.Info().
		Str("email_log_id", state.EmailLogID.String()).
		Str("to", state.To).
		Str("from", state.From).
		Msg("email queued")

	return nil
}
