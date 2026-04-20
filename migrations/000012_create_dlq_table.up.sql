-- Dead Letter Queue table for permanently failed email tasks
CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Task identification
    task_type VARCHAR(100) NOT NULL,
    task_id VARCHAR(255),
    
    -- Original task data
    payload JSONB NOT NULL,
    
    -- Failure information
    failure_reason TEXT NOT NULL,
    error_code VARCHAR(50),
    smtp_response TEXT,
    
    -- Retry information
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    
    -- Email-specific metadata (for quick filtering)
    email_log_id UUID,
    user_id UUID,
    recipient_email VARCHAR(255),
    recipient_domain VARCHAR(255),
    
    -- Timestamps
    failed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Retry tracking
    retried_at TIMESTAMP,
    retry_status VARCHAR(50), -- 'pending', 'retrying', 'success', 'failed_again'
    
    -- Indexes for common queries
    CONSTRAINT dlq_task_id_unique UNIQUE (task_id)
);

-- Indexes for performance
CREATE INDEX idx_dlq_task_type ON dead_letter_queue(task_type);
CREATE INDEX idx_dlq_failed_at ON dead_letter_queue(failed_at DESC);
CREATE INDEX idx_dlq_email_log_id ON dead_letter_queue(email_log_id);
CREATE INDEX idx_dlq_user_id ON dead_letter_queue(user_id);
CREATE INDEX idx_dlq_recipient_domain ON dead_letter_queue(recipient_domain);
CREATE INDEX idx_dlq_error_code ON dead_letter_queue(error_code);
CREATE INDEX idx_dlq_retry_status ON dead_letter_queue(retry_status);

-- Composite index for admin dashboard queries
CREATE INDEX idx_dlq_dashboard ON dead_letter_queue(failed_at DESC, task_type, retry_status);

-- Add comment for documentation
COMMENT ON TABLE dead_letter_queue IS 'Stores permanently failed email tasks for manual inspection and retry';
