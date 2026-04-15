package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for SwiftMail.
type Metrics struct {
	// API Metrics
	APIRequestsTotal    *prometheus.CounterVec
	APIRequestDuration  *prometheus.HistogramVec

	// Email Metrics
	EmailsQueuedTotal   *prometheus.CounterVec
	EmailsSentTotal     *prometheus.CounterVec
	EmailDeliveryDuration prometheus.Histogram

	// SMTP Pool
	SMTPPoolConnections *prometheus.GaugeVec
	SMTPPoolReuseTotal  prometheus.Counter

	// Queue
	QueueDepth *prometheus.GaugeVec

	// Rates
	BounceRate    *prometheus.GaugeVec
	ComplaintRate *prometheus.GaugeVec

	// Redis
	RedisHitRate *prometheus.GaugeVec

	// ClickHouse
	ClickHouseBatchSize    prometheus.Histogram
	ClickHouseFlushDuration prometheus.Histogram
	ClickHouseErrors       prometheus.Counter
}

// NewMetrics registers and returns all Prometheus metrics.
func NewMetrics() *Metrics {
	return &Metrics{
		APIRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "api_requests_total",
				Help:      "Total number of API requests",
			},
			[]string{"method", "path", "status"},
		),
		APIRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "api_request_duration_seconds",
				Help:      "API request duration in seconds",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.08, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"method", "path"},
		),
		EmailsQueuedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "emails_queued_total",
				Help:      "Total number of emails queued",
			},
			[]string{"priority"},
		),
		EmailsSentTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "emails_sent_total",
				Help:      "Total emails sent by status",
			},
			[]string{"status", "domain"},
		),
		EmailDeliveryDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "email_delivery_duration_seconds",
				Help:      "Email delivery duration from queue to SMTP",
				Buckets:   []float64{0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
		),
		SMTPPoolConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "smtp_pool_connections",
				Help:      "Number of SMTP pool connections by state",
			},
			[]string{"ip", "state"},
		),
		SMTPPoolReuseTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "smtp_pool_reuse_total",
				Help:      "Total times an SMTP connection was reused",
			},
		),
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "queue_depth",
				Help:      "Current queue depth by priority",
			},
			[]string{"priority"},
		),
		BounceRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "bounce_rate",
				Help:      "Bounce rate per user",
			},
			[]string{"user_id"},
		),
		ComplaintRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "complaint_rate",
				Help:      "Complaint rate per user",
			},
			[]string{"user_id"},
		),
		RedisHitRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "redis_hit_rate",
				Help:      "Redis cache hit rate by operation",
			},
			[]string{"operation"},
		),
		ClickHouseBatchSize: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_batch_size",
				Help:      "ClickHouse batch insert size",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000},
			},
		),
		ClickHouseFlushDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_flush_duration_seconds",
				Help:      "ClickHouse batch flush duration",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
		),
		ClickHouseErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_errors_total",
				Help:      "Total ClickHouse errors",
			},
		),
	}
}
