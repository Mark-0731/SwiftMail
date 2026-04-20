package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// AuditLogger handles authentication audit logging.
type AuditLogger struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(db *pgxpool.Pool, logger zerolog.Logger) *AuditLogger {
	return &AuditLogger{
		db:     db,
		logger: logger,
	}
}

// LogLoginAttempt logs a login attempt (success or failure).
func (a *AuditLogger) LogLoginAttempt(ctx context.Context, email, ipAddress, userAgent string, success bool, failureReason string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error().Interface("panic", r).Msg("panic in audit logging")
			}
		}()

		_, err := a.db.Exec(context.Background(),
			`INSERT INTO login_attempts (email, ip_address, user_agent, success, failure_reason)
			 VALUES ($1, $2, $3, $4, $5)`,
			email, ipAddress, userAgent, success, failureReason,
		)
		if err != nil {
			a.logger.Error().Err(err).Msg("failed to log login attempt")
		}
	}()
}

// LogAuthEvent logs an authentication-related event to audit_logs table.
func (a *AuditLogger) LogAuthEvent(ctx context.Context, userID *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]interface{}, ipAddress, userAgent string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error().Interface("panic", r).Msg("panic in audit logging")
			}
		}()

		_, err := a.db.Exec(context.Background(),
			`INSERT INTO audit_logs (user_id, action, resource_type, resource_id, metadata, ip_address, user_agent)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			userID, action, resourceType, resourceID, metadata, ipAddress, userAgent,
		)
		if err != nil {
			a.logger.Error().Err(err).Msg("failed to log auth event")
		}
	}()
}

// GetLoginHistory retrieves login history for a user.
func (a *AuditLogger) GetLoginHistory(ctx context.Context, email string, limit int) ([]LoginAttempt, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := a.db.Query(ctx,
		`SELECT id, email, ip_address, user_agent, success, failure_reason, created_at
		 FROM login_attempts
		 WHERE email = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		email, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []LoginAttempt
	for rows.Next() {
		var attempt LoginAttempt
		if err := rows.Scan(
			&attempt.ID,
			&attempt.Email,
			&attempt.IPAddress,
			&attempt.UserAgent,
			&attempt.Success,
			&attempt.FailureReason,
			&attempt.CreatedAt,
		); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}

	return attempts, nil
}

// LoginAttempt represents a login attempt record.
type LoginAttempt struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	IPAddress     *string   `json:"ip_address"`
	UserAgent     *string   `json:"user_agent"`
	Success       bool      `json:"success"`
	FailureReason *string   `json:"failure_reason"`
	CreatedAt     time.Time `json:"created_at"`
}
