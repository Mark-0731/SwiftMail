-- Rollback complete database schema

-- Drop all tables in reverse order of dependencies
DROP TABLE IF EXISTS provider_health_metrics CASCADE;
DROP TABLE IF EXISTS adaptive_retry_strategies CASCADE;
DROP TABLE IF EXISTS poison_queue CASCADE;
DROP TABLE IF EXISTS dlq_retry_history CASCADE;
DROP TABLE IF EXISTS circuit_breaker_state CASCADE;
DROP TABLE IF EXISTS dead_letter_queue CASCADE;
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS credit_transactions CASCADE;
DROP TABLE IF EXISTS credits CASCADE;
DROP TABLE IF EXISTS billing_plans CASCADE;
DROP TABLE IF EXISTS bounce_logs CASCADE;
DROP TABLE IF EXISTS ip_addresses CASCADE;
DROP TABLE IF EXISTS suppression_list CASCADE;
DROP TABLE IF EXISTS webhook_logs CASCADE;
DROP TABLE IF EXISTS webhooks CASCADE;
DROP TABLE IF EXISTS click_events CASCADE;
DROP TABLE IF EXISTS email_events CASCADE;
DROP TABLE IF EXISTS email_logs CASCADE;
DROP TABLE IF EXISTS template_versions CASCADE;
DROP TABLE IF EXISTS templates CASCADE;
DROP TABLE IF EXISTS sender_emails CASCADE;
DROP TABLE IF EXISTS domains CASCADE;
DROP TABLE IF EXISTS login_attempts CASCADE;
DROP TABLE IF EXISTS email_verification_tokens CASCADE;
DROP TABLE IF EXISTS password_reset_tokens CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Drop extensions
DROP EXTENSION IF EXISTS "pgcrypto";
