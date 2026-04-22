package metrics

import (
	"sync"

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

var (
	instance *Metrics
	once     sync.Once
)

// NewMetrics returns a singleton instance of metrics with a custom registry.
// This ensures metrics are only registered once across API and Worker.
func NewMetrics() *Metrics {
	once.Do(func() {
		// Create a new registry to avoid conflicts with default registry
		registry := prometheus.NewRegistry()

		instance = &Metrics{
			Registry: registry,
		}

		instance.APIRequestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "api_requests_total",
				Help:      "Total number of API requests",
			},
			[]string{"method", "path", "status"},
		)
		registry.MustRegister(instance.APIRequestsTotal)

		instance.APIRequestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "api_request_duration_seconds",
				Help:      "API request duration in seconds",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.08, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"method", "path"},
		)
		registry.MustRegister(instance.APIRequestDuration)

		instance.EmailsQueuedTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "emails_queued_total",
				Help:      "Total number of emails queued",
			},
			[]string{"priority"},
		)
		registry.MustRegister(instance.EmailsQueuedTotal)

		instance.EmailsSentTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "emails_sent_total",
				Help:      "Total emails sent by status",
			},
			[]string{"status", "domain"},
		)
		registry.MustRegister(instance.EmailsSentTotal)

		instance.EmailDeliveryDuration = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "email_delivery_duration_seconds",
				Help:      "Email delivery duration from queue to SMTP",
				Buckets:   []float64{0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
		)
		registry.MustRegister(instance.EmailDeliveryDuration)

		instance.SMTPPoolConnections = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "smtp_pool_connections",
				Help:      "Number of SMTP pool connections by state",
			},
			[]string{"ip", "state"},
		)
		registry.MustRegister(instance.SMTPPoolConnections)

		instance.SMTPPoolReuseTotal = prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "smtp_pool_reuse_total",
				Help:      "Total times an SMTP connection was reused",
			},
		)
		registry.MustRegister(instance.SMTPPoolReuseTotal)

		instance.QueueDepth = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "queue_depth",
				Help:      "Current queue depth by priority",
			},
			[]string{"priority"},
		)
		registry.MustRegister(instance.QueueDepth)

		instance.BounceRate = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "bounce_rate",
				Help:      "Bounce rate per user",
			},
			[]string{"user_id"},
		)
		registry.MustRegister(instance.BounceRate)

		instance.ComplaintRate = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "complaint_rate",
				Help:      "Complaint rate per user",
			},
			[]string{"user_id"},
		)
		registry.MustRegister(instance.ComplaintRate)

		instance.RedisHitRate = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "redis_hit_rate",
				Help:      "Redis cache hit rate by operation",
			},
			[]string{"operation"},
		)
		registry.MustRegister(instance.RedisHitRate)

		instance.ClickHouseBatchSize = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_batch_size",
				Help:      "ClickHouse batch insert size",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000},
			},
		)
		registry.MustRegister(instance.ClickHouseBatchSize)

		instance.ClickHouseFlushDuration = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_flush_duration_seconds",
				Help:      "ClickHouse batch flush duration",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
		)
		registry.MustRegister(instance.ClickHouseFlushDuration)

		instance.ClickHouseErrors = prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "clickhouse_errors_total",
				Help:      "Total ClickHouse errors",
			},
		)
		registry.MustRegister(instance.ClickHouseErrors)

		instance.DLQEntriesTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "dlq_entries_total",
				Help:      "Total number of tasks sent to DLQ",
			},
			[]string{"task_type", "error_code"},
		)
		registry.MustRegister(instance.DLQEntriesTotal)

		instance.DLQRetryAttemptsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "dlq_retry_attempts_total",
				Help:      "Total DLQ retry attempts",
			},
			[]string{"task_type", "result"},
		)
		registry.MustRegister(instance.DLQRetryAttemptsTotal)

		instance.DLQRetrySuccessTotal = prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "dlq_retry_success_total",
				Help:      "Total successful DLQ retries",
			},
		)
		registry.MustRegister(instance.DLQRetrySuccessTotal)

		instance.DLQRetryFailureTotal = prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "dlq_retry_failure_total",
				Help:      "Total failed DLQ retries",
			},
		)
		registry.MustRegister(instance.DLQRetryFailureTotal)

		instance.DLQProcessingDuration = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "dlq_processing_duration_seconds",
				Help:      "DLQ entry processing duration",
				Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
			},
		)
		registry.MustRegister(instance.DLQProcessingDuration)

		instance.CircuitBreakerState = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"resource_type", "resource_id"},
		)
		registry.MustRegister(instance.CircuitBreakerState)

		instance.CircuitBreakerTrips = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "circuit_breaker_trips_total",
				Help:      "Total circuit breaker trips",
			},
			[]string{"resource_type", "resource_id"},
		)
		registry.MustRegister(instance.CircuitBreakerTrips)

		instance.PoisonQueueSize = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "swiftmail",
				Name:      "poison_queue_size",
				Help:      "Current number of messages in poison queue",
			},
		)
		registry.MustRegister(instance.PoisonQueueSize)

		instance.TaskProcessingDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "swiftmail",
				Name:      "task_processing_duration_seconds",
				Help:      "Task processing duration by type",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
			},
			[]string{"task_type"},
		)
		registry.MustRegister(instance.TaskProcessingDuration)

		instance.TaskFailuresTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "swiftmail",
				Name:      "task_failures_total",
				Help:      "Total task failures by type and error category",
			},
			[]string{"task_type", "error_category"},
		)
		registry.MustRegister(instance.TaskFailuresTotal)
	})

	return instance
}
