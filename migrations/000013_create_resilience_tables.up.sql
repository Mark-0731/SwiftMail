-- Circuit Breaker State Table
CREATE TABLE IF NOT EXISTS circuit_breaker_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL, -- 'provider', 'domain'
    resource_id VARCHAR(255) NOT NULL,
    state VARCHAR(20) NOT NULL, -- 'closed', 'open', 'half_open'
    failure_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    last_failure_at TIMESTAMP,
    opened_at TIMESTAMP,
    half_open_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    CONSTRAINT circuit_breaker_unique UNIQUE (resource_type, resource_id)
);

CREATE INDEX idx_circuit_breaker_resource ON circuit_breaker_state(resource_type, resource_id);
CREATE INDEX idx_circuit_breaker_state ON circuit_breaker_state(state);
CREATE INDEX idx_circuit_breaker_updated ON circuit_breaker_state(updated_at DESC);

COMMENT ON TABLE circuit_breaker_state IS 'Tracks circuit breaker state for providers and domains';

-- DLQ Retry History Table
CREATE TABLE IF NOT EXISTS dlq_retry_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dlq_entry_id UUID NOT NULL REFERENCES dead_letter_queue(id) ON DELETE CASCADE,
    retry_attempt INTEGER NOT NULL,
    retried_at TIMESTAMP NOT NULL DEFAULT NOW(),
    retry_result VARCHAR(20) NOT NULL, -- 'success', 'failed', 'deferred'
    error_message TEXT,
    duration_ms INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlq_retry_history_entry ON dlq_retry_history(dlq_entry_id);
CREATE INDEX idx_dlq_retry_history_retried ON dlq_retry_history(retried_at DESC);
CREATE INDEX idx_dlq_retry_history_result ON dlq_retry_history(retry_result);

COMMENT ON TABLE dlq_retry_history IS 'Tracks retry attempts for DLQ entries';

-- Poison Queue Table
CREATE TABLE IF NOT EXISTS poison_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_dlq_id UUID NOT NULL,
    task_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    failure_pattern TEXT NOT NULL,
    repeated_failure_count INTEGER NOT NULL,
    first_failed_at TIMESTAMP NOT NULL,
    last_failed_at TIMESTAMP NOT NULL,
    quarantined_at TIMESTAMP NOT NULL DEFAULT NOW(),
    reviewed BOOLEAN NOT NULL DEFAULT FALSE,
    review_notes TEXT,
    reviewed_by UUID,
    reviewed_at TIMESTAMP,
    resolution_action VARCHAR(50), -- 'retry', 'discard', 'manual_fix'
    
    -- Metadata for analysis
    email_log_id UUID,
    user_id UUID,
    recipient_email VARCHAR(255),
    recipient_domain VARCHAR(255),
    error_codes TEXT[], -- Array of error codes
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_poison_queue_reviewed ON poison_queue(reviewed);
CREATE INDEX idx_poison_queue_quarantined ON poison_queue(quarantined_at DESC);
CREATE INDEX idx_poison_queue_email_log ON poison_queue(email_log_id);
CREATE INDEX idx_poison_queue_user ON poison_queue(user_id);
CREATE INDEX idx_poison_queue_domain ON poison_queue(recipient_domain);
CREATE INDEX idx_poison_queue_pattern ON poison_queue(failure_pattern);

COMMENT ON TABLE poison_queue IS 'Isolates messages that fail repeatedly for manual review';

-- Adaptive Retry Strategies Table
CREATE TABLE IF NOT EXISTS adaptive_retry_strategies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain VARCHAR(255) NOT NULL,
    error_category VARCHAR(100) NOT NULL,
    base_delay_ms BIGINT NOT NULL,
    max_delay_ms BIGINT NOT NULL,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.5,
    average_retries DECIMAL(5,2) NOT NULL DEFAULT 0,
    sample_size INTEGER NOT NULL DEFAULT 0,
    optimal_time_slots INTEGER[], -- Hours of day (0-23)
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    CONSTRAINT adaptive_retry_unique UNIQUE (domain, error_category)
);

CREATE INDEX idx_adaptive_retry_domain ON adaptive_retry_strategies(domain);
CREATE INDEX idx_adaptive_retry_category ON adaptive_retry_strategies(error_category);
CREATE INDEX idx_adaptive_retry_updated ON adaptive_retry_strategies(last_updated DESC);

COMMENT ON TABLE adaptive_retry_strategies IS 'Learned retry strategies based on historical patterns';

-- Provider Health Metrics Table
CREATE TABLE IF NOT EXISTS provider_health_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_name VARCHAR(100) NOT NULL,
    metric_type VARCHAR(50) NOT NULL, -- 'success_rate', 'latency_p50', 'latency_p95', 'error_rate'
    metric_value DECIMAL(10,4) NOT NULL,
    window_start TIMESTAMP NOT NULL,
    window_end TIMESTAMP NOT NULL,
    sample_count INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_provider_health_provider ON provider_health_metrics(provider_name);
CREATE INDEX idx_provider_health_type ON provider_health_metrics(metric_type);
CREATE INDEX idx_provider_health_window ON provider_health_metrics(window_start DESC, window_end DESC);

COMMENT ON TABLE provider_health_metrics IS 'Tracks provider health metrics over time';

-- Add indexes to existing DLQ table for better performance
CREATE INDEX IF NOT EXISTS idx_dlq_failed_at_status ON dead_letter_queue(failed_at DESC, retry_status);
CREATE INDEX IF NOT EXISTS idx_dlq_email_log_failed ON dead_letter_queue(email_log_id, failed_at DESC);
