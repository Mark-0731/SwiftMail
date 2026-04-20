package events

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Bus defines the event bus interface.
type Bus interface {
	Publish(ctx context.Context, event *Event) error
}

// RedisBus implements Bus using Redis Streams.
type RedisBus struct {
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewRedisBus creates a new Redis-based event bus.
func NewRedisBus(rdb *redis.Client, logger zerolog.Logger) Bus {
	return &RedisBus{
		rdb:    rdb,
		logger: logger,
	}
}

// Publish publishes an event to Redis Stream.
func (b *RedisBus) Publish(ctx context.Context, event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	streamKey := "events:" + event.Type
	_, err = b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"event": string(data),
		},
	}).Result()

	if err != nil {
		b.logger.Error().Err(err).Str("event_type", event.Type).Msg("failed to publish event")
		return err
	}

	b.logger.Debug().Str("event_type", event.Type).Str("event_id", event.ID).Msg("event published")
	return nil
}
