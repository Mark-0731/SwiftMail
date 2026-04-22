package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for SwiftMail.
type Metrics struct {
	Registry *prometheus.Registry

	// API Metrics
	APIRequestsTotal   *prometheus.CounterVec
	APIRequestDuration *prometheus.HistogramVec

	// Email Metrics
	EmailsQueuedTotal     *prometheus.CounterVec
	EmailsSentTotal       *prometheus.CounterVec
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
	ClickHouseBatchSize     prometheus.Histogram
	ClickHouseFlushDuration prometheus.Histogram
	ClickHouseErrors        prometheus.Counter

	// DLQ & Resilience Metrics
	DLQEntriesTotal        *prometheus.CounterVec
	DLQRetryAttemptsTotal  *prometheus.CounterVec
	DLQRetrySuccessTotal   prometheus.Counter
	DLQRetryFailureTotal   prometheus.Counter
	DLQProcessingDuration  prometheus.Histogram
	CircuitBreakerState    *prometheus.GaugeVec
	CircuitBreakerTrips    *prometheus.CounterVec
	PoisonQueueSize        prometheus.Gauge
	TaskProcessingDuration *prometheus.HistogramVec
	TaskFailuresTotal      *prometheus.CounterVec
}

// NewMetrics creates a new metrics instance with its own registry.
// Each process (API, Worker) should call this once.
func NewMetrics() *Metrics {
	// Create a new registry for this instance
	registry := prometheus.NewRegistry()

	m := &Metrics{
		Registry: registry,
	}

	m.APIRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "api_requests_total",
			Help:      "Total number of API requests",
		},
		[]string{"method", "path", "status"},
	)
	registry.MustRegister(m.APIRequestsTotal)

	m.APIRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "api_request_duration_seconds",
			Help:      "API request duration in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.08, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"method", "path"},
	)
	registry.MustRegister(m.APIRequestDuration)

	m.EmailsQueuedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "emails_queued_total",
			Help:      "Total number of emails queued",
		},
		[]string{"priority"},
	)
	registry.MustRegister(m.EmailsQueuedTotal)

	m.EmailsSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "emails_sent_total",
			Help:      "Total emails sent by status",
		},
		[]string{"status", "domain"},
	)
	registry.MustRegister(m.EmailsSentTotal)

	m.EmailDeliveryDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "email_delivery_duration_seconds",
			Help:      "Email delivery duration from queue to SMTP",
			Buckets:   []float64{0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
	)
	registry.MustRegister(m.EmailDeliveryDuration)

	m.SMTPPoolConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "smtp_pool_connections",
			Help:      "Number of SMTP pool connections by state",
		},
		[]string{"ip", "state"},
	)
	registry.MustRegister(m.SMTPPoolConnections)

	m.SMTPPoolReuseTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "smtp_pool_reuse_total",
			Help:      "Total times an SMTP connection was reused",
		},
	)
	registry.MustRegister(m.SMTPPoolReuseTotal)

	m.QueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "queue_depth",
			Help:      "Current queue depth by priority",
		},
		[]string{"priority"},
	)
	registry.MustRegister(m.QueueDepth)

	m.BounceRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "bounce_rate",
			Help:      "Bounce rate per user",
		},
		[]string{"user_id"},
	)
	registry.MustRegister(m.BounceRate)

	m.ComplaintRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "complaint_rate",
			Help:      "Complaint rate per user",
		},
		[]string{"user_id"},
	)
	registry.MustRegister(m.ComplaintRate)

	m.RedisHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "redis_hit_rate",
			Help:      "Redis cache hit rate by operation",
		},
		[]string{"operation"},
	)
	registry.MustRegister(m.RedisHitRate)

	m.ClickHouseBatchSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "clickhouse_batch_size",
			Help:      "ClickHouse batch insert size",
			Buckets:   []float64{10, 50, 100, 250, 500, 1000},
		},
	)
	registry.MustRegister(m.ClickHouseBatchSize)

	m.ClickHouseFlushDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "clickhouse_flush_duration_seconds",
			Help:      "ClickHouse batch flush duration",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
	)
	registry.MustRegister(m.ClickHouseFlushDuration)

	m.ClickHouseErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "clickhouse_errors_total",
			Help:      "Total ClickHouse errors",
		},
	)
	registry.MustRegister(m.ClickHouseErrors)

	m.DLQEntriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "dlq_entries_total",
			Help:      "Total number of tasks sent to DLQ",
		},
		[]string{"task_type", "error_code"},
	)
	registry.MustRegister(m.DLQEntriesTotal)

	m.DLQRetryAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "dlq_retry_attempts_total",
			Help:      "Total DLQ retry attempts",
		},
		[]string{"task_type", "result"},
	)
	registry.MustRegister(m.DLQRetryAttemptsTotal)

	m.DLQRetrySuccessTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "dlq_retry_success_total",
			Help:      "Total successful DLQ retries",
		},
	)
	registry.MustRegister(m.DLQRetrySuccessTotal)

	m.DLQRetryFailureTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "dlq_retry_failure_total",
			Help:      "Total failed DLQ retries",
		},
	)
	registry.MustRegister(m.DLQRetryFailureTotal)

	m.DLQProcessingDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "dlq_processing_duration_seconds",
			Help:      "DLQ entry processing duration",
			Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
		},
	)
	registry.MustRegister(m.DLQProcessingDuration)

	m.CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"resource_type", "resource_id"},
	)
	registry.MustRegister(m.CircuitBreakerState)

	m.CircuitBreakerTrips = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "circuit_breaker_trips_total",
			Help:      "Total circuit breaker trips",
		},
		[]string{"resource_type", "resource_id"},
	)
	registry.MustRegister(m.CircuitBreakerTrips)

	m.PoisonQueueSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "poison_queue_size",
			Help:      "Current number of messages in poison queue",
		},
	)
	registry.MustRegister(m.PoisonQueueSize)

	m.TaskProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "swiftmail",
			Name:      "task_processing_duration_seconds",
			Help:      "Task processing duration by type",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
		},
		[]string{"task_type"},
	)
	registry.MustRegister(m.TaskProcessingDuration)

	m.TaskFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "swiftmail",
			Name:      "task_failures_total",
			Help:      "Total task failures by type and error category",
		},
		[]string{"task_type", "error_category"},
	)
	registry.MustRegister(m.TaskFailuresTotal)

	return m
}

// InflightGauge tracks the number of in-flight requests.
// This is useful for monitoring concurrent request load.
type InflightGauge struct {
	gauge *prometheus.GaugeVec
}

// NewInflightGauge creates a new in-flight request gauge.
func NewInflightGauge(registry *prometheus.Registry) *InflightGauge {
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "swiftmail",
			Name:      "api_requests_inflight",
			Help:      "Number of in-flight API requests",
		},
		[]string{"method", "path"},
	)
	registry.MustRegister(gauge)

	return &InflightGauge{gauge: gauge}
}

// Inc increments the in-flight counter for a given method and path.
func (ig *InflightGauge) Inc(method, path string) {
	ig.gauge.WithLabelValues(method, path).Inc()
}

// Dec decrements the in-flight counter for a given method and path.
func (ig *InflightGauge) Dec(method, path string) {
	ig.gauge.WithLabelValues(method, path).Dec()
}
