-- Drop indexes from DLQ table
DROP INDEX IF EXISTS idx_dlq_email_log_failed;
DROP INDEX IF EXISTS idx_dlq_failed_at_status;

-- Drop provider health metrics table
DROP TABLE IF EXISTS provider_health_metrics;

-- Drop adaptive retry strategies table
DROP TABLE IF EXISTS adaptive_retry_strategies;

-- Drop poison queue table
DROP TABLE IF EXISTS poison_queue;

-- Drop DLQ retry history table
DROP TABLE IF EXISTS dlq_retry_history;

-- Drop circuit breaker state table
DROP TABLE IF EXISTS circuit_breaker_state;
