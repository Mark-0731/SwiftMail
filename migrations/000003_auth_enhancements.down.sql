-- Migration 000003 Rollback: Authentication Enhancements

DROP TABLE IF EXISTS login_attempts;
DROP TABLE IF EXISTS email_verification_tokens;
DROP TABLE IF EXISTS password_reset_tokens;

ALTER TABLE users DROP COLUMN IF EXISTS token_version;
