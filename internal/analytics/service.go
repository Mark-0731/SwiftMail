package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Service provides analytics query capabilities.
type Service struct {
	conn    clickhouse.Conn
	batcher *Batcher
	logger  zerolog.Logger
}

// NewService creates an analytics service with a batcher for writes and conn for reads.
func NewService(conn clickhouse.Conn, logger zerolog.Logger) *Service {
	return &Service{
		conn:    conn,
		batcher: NewBatcher(conn, logger),
		logger:  logger,
	}
}

// GetBatcher returns the batcher for event ingestion.
func (s *Service) GetBatcher() *Batcher {
	return s.batcher
}

// Stop stops the batcher.
func (s *Service) Stop() {
	s.batcher.Stop()
}

// TrackEvent adds an event through the batcher.
func (s *Service) TrackEvent(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	s.batcher.Push(event)
}

// GetOverview returns aggregated stats for a user in a time range.
func (s *Service) GetOverview(ctx context.Context, userID uuid.UUID, from, to time.Time) (*Overview, error) {
	overview := &Overview{}

	query := `
		SELECT
			countIf(event_type = 'sent')       AS sent,
			countIf(event_type = 'delivered')   AS delivered,
			countIf(event_type = 'opened')      AS opened,
			countIf(event_type = 'clicked')     AS clicked,
			countIf(event_type = 'bounced')     AS bounced,
			countIf(event_type = 'complained')  AS complained
		FROM email_events
		WHERE user_id = ? AND timestamp >= ? AND timestamp <= ?
	`

	err := s.conn.QueryRow(ctx, query, userID, from, to).Scan(
		&overview.Sent, &overview.Delivered, &overview.Opened,
		&overview.Clicked, &overview.Bounced, &overview.Complained,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics query failed: %w", err)
	}

	// Calculate rates
	if overview.Sent > 0 {
		overview.DeliveryRate = float64(overview.Delivered) / float64(overview.Sent) * 100
		overview.OpenRate = float64(overview.Opened) / float64(overview.Sent) * 100
		overview.ClickRate = float64(overview.Clicked) / float64(overview.Sent) * 100
		overview.BounceRate = float64(overview.Bounced) / float64(overview.Sent) * 100
	}

	return overview, nil
}

// GetTimeSeries returns daily aggregated stats.
func (s *Service) GetTimeSeries(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]TimeSeriesPoint, error) {
	query := `
		SELECT
			toDate(timestamp) AS day,
			countIf(event_type = 'sent')     AS sent,
			countIf(event_type = 'delivered') AS delivered,
			countIf(event_type = 'opened')   AS opened,
			countIf(event_type = 'clicked')  AS clicked,
			countIf(event_type = 'bounced')  AS bounced
		FROM email_events
		WHERE user_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY day ORDER BY day
	`

	rows, err := s.conn.Query(ctx, query, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		p := TimeSeriesPoint{}
		var day time.Time
		if err := rows.Scan(&day, &p.Sent, &p.Delivered, &p.Opened, &p.Clicked, &p.Bounced); err != nil {
			return nil, err
		}
		p.Time = day.Format("2006-01-02")
		points = append(points, p)
	}

	return points, nil
}

// GetTopDomains returns top recipient domains by volume.
func (s *Service) GetTopDomains(ctx context.Context, userID uuid.UUID, from, to time.Time, limit int) ([]DomainStats, error) {
	query := `
		SELECT
			domain(recipient) AS dom,
			countIf(event_type = 'sent') AS sent,
			countIf(event_type = 'delivered') AS delivered,
			countIf(event_type = 'bounced') AS bounced
		FROM email_events
		WHERE user_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY dom ORDER BY sent DESC LIMIT ?
	`

	rows, err := s.conn.Query(ctx, query, userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DomainStats
	for rows.Next() {
		d := DomainStats{}
		var bounced int64
		if err := rows.Scan(&d.Domain, &d.Sent, &d.Delivered, &bounced); err != nil {
			return nil, err
		}
		if d.Sent > 0 {
			d.BounceRate = float64(bounced) / float64(d.Sent) * 100
		}
		stats = append(stats, d)
	}

	return stats, nil
}
