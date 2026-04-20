package application

import (
	"context"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/features/analytics"
)

// Batcher buffers analytics events and flushes them to ClickHouse in batches.
type Batcher struct {
	conn      clickhouse.Conn
	logger    zerolog.Logger
	buffer    []analytics.Event
	mu        sync.Mutex
	maxSize   int
	flushTick time.Duration
	done      chan struct{}
}

// NewBatcher creates a batcher that flushes every second or at 1000 events.
func NewBatcher(conn clickhouse.Conn, logger zerolog.Logger) *Batcher {
	b := &Batcher{
		conn:      conn,
		logger:    logger,
		buffer:    make([]analytics.Event, 0, 1000),
		maxSize:   1000,
		flushTick: 1 * time.Second,
		done:      make(chan struct{}),
	}
	go b.run()
	return b
}

// Push adds an event to the buffer.
func (b *Batcher) Push(event analytics.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = append(b.buffer, event)
	if len(b.buffer) >= b.maxSize {
		go b.flush()
	}
}

// Stop stops the batcher and flushes remaining events.
func (b *Batcher) Stop() {
	close(b.done)
	b.flush()
}

func (b *Batcher) run() {
	ticker := time.NewTicker(b.flushTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.done:
			return
		}
	}
}

func (b *Batcher) flush() {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return
	}
	events := b.buffer
	b.buffer = make([]analytics.Event, 0, b.maxSize)
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batch, err := b.conn.PrepareBatch(ctx, `INSERT INTO email_events (user_id, email_id, domain_id, event_type, recipient, ip_address, user_agent, timestamp)`)
	if err != nil {
		b.logger.Error().Err(err).Int("count", len(events)).Msg("failed to prepare ClickHouse batch")
		return
	}

	for _, e := range events {
		if err := batch.Append(e.UserID, e.EmailID, e.DomainID, e.EventType, e.Recipient, e.IPAddress, e.UserAgent, e.Timestamp); err != nil {
			b.logger.Error().Err(err).Msg("failed to append to batch")
		}
	}

	if err := batch.Send(); err != nil {
		b.logger.Error().Err(err).Int("count", len(events)).Msg("failed to send ClickHouse batch")
		return
	}

	b.logger.Debug().Int("count", len(events)).Msg("flushed analytics batch to ClickHouse")
}
