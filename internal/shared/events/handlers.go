package events

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/features/webhook/application"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// WebhookHandler handles events by dispatching webhooks.
type WebhookHandler struct {
	dispatcher *application.Dispatcher
	logger     zerolog.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(dispatcher *application.Dispatcher, logger zerolog.Logger) *WebhookHandler {
	return &WebhookHandler{
		dispatcher: dispatcher,
		logger:     logger,
	}
}

// Handle processes an event and dispatches webhooks.
func (h *WebhookHandler) Handle(ctx context.Context, event *Event) error {
	// Extract user ID from event data
	userIDStr, ok := event.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing user_id in event data")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Dispatch webhook (fire and forget)
	h.dispatcher.Dispatch(ctx, userID, event.Type, event.Data)

	h.logger.Debug().Str("event_type", event.Type).Msg("webhook dispatched")
	return nil
}

// AbuseDetectionHandler handles abuse detection on email events.
type AbuseDetectionHandler struct {
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewAbuseDetectionHandler creates a new abuse detection handler.
func NewAbuseDetectionHandler(rdb *redis.Client, logger zerolog.Logger) *AbuseDetectionHandler {
	return &AbuseDetectionHandler{
		rdb:    rdb,
		logger: logger,
	}
}

// Handle processes email events for abuse detection.
func (h *AbuseDetectionHandler) Handle(ctx context.Context, event *Event) error {
	userIDStr, ok := event.Data["user_id"].(string)
	if !ok {
		return nil
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil
	}

	// Track email counts in Redis with 24-hour expiry
	key := fmt.Sprintf("abuse:%s", userID.String())

	switch event.Type {
	case EmailSent:
		h.rdb.HIncrBy(ctx, key, "sent", 1)
	case EmailFailed:
		h.rdb.HIncrBy(ctx, key, "bounced", 1)
	case EmailBounced:
		h.rdb.HIncrBy(ctx, key, "bounced", 1)
	}

	// Set expiry on first write
	h.rdb.Expire(ctx, key, 24*time.Hour)

	// Check abuse thresholds
	counts, err := h.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return err
	}

	sent := parseInt(counts["sent"])
	bounced := parseInt(counts["bounced"])

	// Abuse detection rules
	if bounced > 100 && sent > 0 {
		bounceRate := float64(bounced) / float64(sent)
		if bounceRate > 0.1 { // 10% bounce rate
			h.logger.Warn().
				Str("user_id", userID.String()).
				Int64("sent", sent).
				Int64("bounced", bounced).
				Float64("bounce_rate", bounceRate).
				Msg("high bounce rate detected - potential abuse")
		}
	}

	// Suspicious volume
	if sent > 10000 {
		h.logger.Warn().
			Str("user_id", userID.String()).
			Int64("sent", sent).
			Msg("high volume detected in 24h")
	}

	return nil
}

func parseInt(s string) int64 {
	if s == "" {
		return 0
	}
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}
