-- SwiftMail Database Schema - Complete Migration
-- Consolidated from all migrations for fresh database setup

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ==================== USERS & AUTH ====================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(20) DEFAULT 'owner' CHECK (role IN ('owner', 'developer', 'viewer')),
    totp_secret VARCHAR(255),
    totp_enabled BOOLEAN DEFAULT FALSE,
    email_verified BOOLEAN DEFAULT FALSE,
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'warned', 'throttled', 'suspended', 'banned')),
    stripe_customer_id VARCHAR(255),
    suspended_at TIMESTAMPTZ,
    token_version INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_users_stripe_customer ON users(stripe_customer_id);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(12) NOT NULL,
    permissions JSONB DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

-- Password reset tokens table
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);
CREATE INDEX idx_password_reset_tokens_expires ON password_reset_tokens(expires_at);

-- Email verification tokens table
CREATE TABLE email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_email_verification_tokens_user ON email_verification_tokens(user_id);
CREATE INDEX idx_email_verification_tokens_hash ON email_verification_tokens(token_hash);
CREATE INDEX idx_email_verification_tokens_expires ON email_verification_tokens(expires_at);

-- Login attempts tracking table
CREATE TABLE login_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45),
    user_agent TEXT,
    success BOOLEAN NOT NULL,
    failure_reason VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_login_attempts_email ON login_attempts(email, created_at);
CREATE INDEX idx_login_attempts_ip ON login_attempts(ip_address, created_at);

-- ==================== DOMAINS & SENDERS ====================

CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'failed')),
    spf_record TEXT,
    dkim_public_key TEXT,
    dkim_private_key_encrypted BYTEA,
    dkim_selector VARCHAR(63),
    dmarc_record TEXT,
    bimi_logo_url TEXT,
    bimi_vmc_url TEXT,
    mx_verified BOOLEAN DEFAULT FALSE,
    warmup_day INT DEFAULT 0,
    warmup_active BOOLEAN DEFAULT FALSE,
    last_verified_at TIMESTAMPTZ,
    dkim_rotated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, domain)
);
CREATE INDEX idx_domains_user ON domains(user_id);
CREATE INDEX idx_domains_domain ON domains(domain);

CREATE TABLE sender_emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    verified BOOLEAN DEFAULT FALSE,
    verification_token VARCHAR(255),
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_sender_emails_user ON sender_emails(user_id);

-- ==================== TEMPLATES ====================

CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    active_version INT DEFAULT 1,
    archived BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_templates_user ON templates(user_id);

CREATE TABLE template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    version INT NOT NULL,
    subject TEXT NOT NULL,
    html_body TEXT,
    text_body TEXT,
    variables JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(template_id, version)
);

-- ==================== EMAIL LOGS ====================

CREATE TABLE email_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    domain_id UUID REFERENCES domains(id),
    idempotency_key VARCHAR(255),
    message_id VARCHAR(255) UNIQUE,
    from_email VARCHAR(255) NOT NULL,
    to_email VARCHAR(255) NOT NULL,
    subject TEXT,
    status VARCHAR(20) DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'sending', 'sent', 'delivered', 'deferred', 'failed', 'bounced', 'complained')),
    previous_status VARCHAR(20),
    status_changed_at TIMESTAMPTZ DEFAULT NOW(),
    template_id UUID,
    tags JSONB DEFAULT '[]',
    ip_used VARCHAR(45),
    smtp_response TEXT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 5,
    attachments JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    opened_at TIMESTAMPTZ,
    clicked_at TIMESTAMPTZ,
    bounced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_email_logs_user_status ON email_logs(user_id, status);
CREATE INDEX idx_email_logs_created_at ON email_logs(created_at);
CREATE INDEX idx_email_logs_to_email ON email_logs(to_email);
CREATE INDEX idx_email_logs_idempotency ON email_logs(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_email_logs_message_id ON email_logs(message_id);
CREATE INDEX idx_email_logs_tags ON email_logs USING gin(tags);

CREATE TABLE email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_log_id UUID NOT NULL REFERENCES email_logs(id) ON DELETE CASCADE,
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('queued', 'processing', 'sending', 'sent', 'delivered', 'deferred', 'failed', 'bounced', 'complained', 'opened', 'clicked', 'unsubscribed')),
    metadata JSONB DEFAULT '{}',
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_email_events_log ON email_events(email_log_id);
CREATE INDEX idx_email_events_type ON email_events(event_type, created_at);

CREATE TABLE click_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_log_id UUID NOT NULL REFERENCES email_logs(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    clicked_at TIMESTAMPTZ DEFAULT NOW(),
    ip_address VARCHAR(45),
    user_agent TEXT,
    country VARCHAR(2),
    city VARCHAR(100)
);
CREATE INDEX idx_click_events_log ON click_events(email_log_id);

-- ==================== WEBHOOKS ====================

CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    secret VARCHAR(255) NOT NULL,
    events JSONB DEFAULT '[]',
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_webhooks_user ON webhooks(user_id);

CREATE TABLE webhook_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(20),
    payload JSONB,
    response_status INT,
    response_body TEXT,
    attempt INT DEFAULT 1,
    success BOOLEAN,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_webhook_logs_webhook ON webhook_logs(webhook_id, created_at);

-- ==================== SUPPRESSION ====================

CREATE TABLE suppression_list (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    email VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('hard_bounce', 'soft_bounce', 'complaint', 'manual', 'unsubscribe')),
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, email)
);
CREATE INDEX idx_suppression_email ON suppression_list(email);
CREATE INDEX idx_suppression_user ON suppression_list(user_id);

-- ==================== IP MANAGEMENT ====================

CREATE TABLE ip_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip VARCHAR(45) NOT NULL UNIQUE,
    type VARCHAR(20) DEFAULT 'shared' CHECK (type IN ('shared', 'dedicated')),
    assigned_user_id UUID REFERENCES users(id),
    health_score INT DEFAULT 100 CHECK (health_score >= 0 AND health_score <= 100),
    warmup_day INT DEFAULT 0,
    warmup_active BOOLEAN DEFAULT FALSE,
    daily_limit INT DEFAULT 50,
    daily_sent INT DEFAULT 0,
    ptr_record VARCHAR(255),
    blacklisted BOOLEAN DEFAULT FALSE,
    last_health_check TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE bounce_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_log_id UUID NOT NULL REFERENCES email_logs(id) ON DELETE CASCADE,
    bounce_type VARCHAR(10) NOT NULL CHECK (bounce_type IN ('hard', 'soft')),
    bounce_code VARCHAR(10),
    diagnostic TEXT,
    recipient VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_bounce_logs_email ON bounce_logs(email_log_id);

-- ==================== BILLING ====================

CREATE TABLE billing_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    monthly_limit BIGINT NOT NULL,
    price_cents BIGINT NOT NULL,
    features JSONB DEFAULT '{}',
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE credits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    balance BIGINT DEFAULT 0,
    auto_topup_enabled BOOLEAN DEFAULT FALSE,
    auto_topup_threshold BIGINT DEFAULT 100,
    auto_topup_amount BIGINT DEFAULT 1000,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE credit_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount BIGINT NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('purchase', 'usage', 'refund', 'bonus')),
    description TEXT,
    stripe_payment_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_credit_tx_user ON credit_transactions(user_id, created_at);
CREATE INDEX idx_credit_tx_stripe ON credit_transactions(stripe_payment_id);

CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    stripe_subscription_id VARCHAR(255) NOT NULL,
    plan_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'canceled', 'past_due', 'trialing', 'incomplete', 'incomplete_expired', 'unpaid')),
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);

-- ==================== AUDIT ====================

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50),
    resource_id UUID,
    metadata JSONB DEFAULT '{}',
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id, created_at);
CREATE INDEX idx_audit_logs_action ON audit_logs(action, created_at);

-- ==================== DEAD LETTER QUEUE ====================

CREATE TABLE dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type VARCHAR(100) NOT NULL,
    task_id VARCHAR(255),
    payload JSONB NOT NULL,
    failure_reason TEXT NOT NULL,
    error_code VARCHAR(50),
    smtp_response TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    email_log_id UUID,
    user_id UUID,
    recipient_email VARCHAR(255),
    recipient_domain VARCHAR(255),
    failed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    retried_at TIMESTAMP,
    retry_status VARCHAR(50),
    CONSTRAINT dlq_task_id_unique UNIQUE (task_id)
);

CREATE INDEX idx_dlq_task_type ON dead_letter_queue(task_type);
CREATE INDEX idx_dlq_failed_at ON dead_letter_queue(failed_at DESC);
CREATE INDEX idx_dlq_email_log_id ON dead_letter_queue(email_log_id);
CREATE INDEX idx_dlq_user_id ON dead_letter_queue(user_id);
CREATE INDEX idx_dlq_recipient_domain ON dead_letter_queue(recipient_domain);
CREATE INDEX idx_dlq_error_code ON dead_letter_queue(error_code);
CREATE INDEX idx_dlq_retry_status ON dead_letter_queue(retry_status);
CREATE INDEX idx_dlq_dashboard ON dead_letter_queue(failed_at DESC, task_type, retry_status);
CREATE INDEX idx_dlq_failed_at_status ON dead_letter_queue(failed_at DESC, retry_status);
CREATE INDEX idx_dlq_email_log_failed ON dead_letter_queue(email_log_id, failed_at DESC);

COMMENT ON TABLE dead_letter_queue IS 'Stores permanently failed email tasks for manual inspection and retry';

-- ==================== RESILIENCE TABLES ====================

-- Circuit Breaker State Table
CREATE TABLE circuit_breaker_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    state VARCHAR(20) NOT NULL,
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
CREATE TABLE dlq_retry_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dlq_entry_id UUID NOT NULL REFERENCES dead_letter_queue(id) ON DELETE CASCADE,
    retry_attempt INTEGER NOT NULL,
    retried_at TIMESTAMP NOT NULL DEFAULT NOW(),
    retry_result VARCHAR(20) NOT NULL,
    error_message TEXT,
    duration_ms INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlq_retry_history_entry ON dlq_retry_history(dlq_entry_id);
CREATE INDEX idx_dlq_retry_history_retried ON dlq_retry_history(retried_at DESC);
CREATE INDEX idx_dlq_retry_history_result ON dlq_retry_history(retry_result);

COMMENT ON TABLE dlq_retry_history IS 'Tracks retry attempts for DLQ entries';

-- Poison Queue Table
CREATE TABLE poison_queue (
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
    resolution_action VARCHAR(50),
    email_log_id UUID,
    user_id UUID,
    recipient_email VARCHAR(255),
    recipient_domain VARCHAR(255),
    error_codes TEXT[],
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
CREATE TABLE adaptive_retry_strategies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain VARCHAR(255) NOT NULL,
    error_category VARCHAR(100) NOT NULL,
    base_delay_ms BIGINT NOT NULL,
    max_delay_ms BIGINT NOT NULL,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.5,
    average_retries DECIMAL(5,2) NOT NULL DEFAULT 0,
    sample_size INTEGER NOT NULL DEFAULT 0,
    optimal_time_slots INTEGER[],
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT adaptive_retry_unique UNIQUE (domain, error_category)
);

CREATE INDEX idx_adaptive_retry_domain ON adaptive_retry_strategies(domain);
CREATE INDEX idx_adaptive_retry_category ON adaptive_retry_strategies(error_category);
CREATE INDEX idx_adaptive_retry_updated ON adaptive_retry_strategies(last_updated DESC);

COMMENT ON TABLE adaptive_retry_strategies IS 'Learned retry strategies based on historical patterns';

-- Provider Health Metrics Table
CREATE TABLE provider_health_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_name VARCHAR(100) NOT NULL,
    metric_type VARCHAR(50) NOT NULL,
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

-- ==================== SEED DATA ====================

INSERT INTO billing_plans (name, monthly_limit, price_cents, features) VALUES
    ('Free', 1000, 0, '{"support": "community", "ips": "shared"}'),
    ('Starter', 50000, 2500, '{"support": "email", "ips": "shared", "custom_domain": true}'),
    ('Pro', 500000, 9900, '{"support": "priority", "ips": "dedicated", "custom_domain": true, "webhooks": true}'),
    ('Enterprise', 0, 0, '{"support": "dedicated", "ips": "dedicated", "custom_domain": true, "webhooks": true, "sla": true}');
