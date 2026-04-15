package analytics

import (
	"time"

	"github.com/google/uuid"
)

// Event represents an analytics event to be batched into ClickHouse.
type Event struct {
	UserID    uuid.UUID `json:"user_id"`
	EmailID   uuid.UUID `json:"email_id"`
	DomainID  uuid.UUID `json:"domain_id"`
	EventType string    `json:"event_type"` // sent, delivered, opened, clicked, bounced, complained
	Recipient string    `json:"recipient"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Timestamp time.Time `json:"timestamp"`
}

// Overview is the response for dashboard overview stats.
type Overview struct {
	Sent       int64   `json:"sent"`
	Delivered  int64   `json:"delivered"`
	Opened     int64   `json:"opened"`
	Clicked    int64   `json:"clicked"`
	Bounced    int64   `json:"bounced"`
	Complained int64   `json:"complained"`
	DeliveryRate float64 `json:"delivery_rate"`
	OpenRate     float64 `json:"open_rate"`
	ClickRate    float64 `json:"click_rate"`
	BounceRate   float64 `json:"bounce_rate"`
}

// TimeSeriesPoint is one data point for a time series.
type TimeSeriesPoint struct {
	Time      string `json:"time"`
	Sent      int64  `json:"sent"`
	Delivered int64  `json:"delivered"`
	Opened    int64  `json:"opened"`
	Clicked   int64  `json:"clicked"`
	Bounced   int64  `json:"bounced"`
}

// Domain stats for top recipients table.
type DomainStats struct {
	Domain    string  `json:"domain"`
	Sent      int64   `json:"sent"`
	Delivered int64   `json:"delivered"`
	BounceRate float64 `json:"bounce_rate"`
}
