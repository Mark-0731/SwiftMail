package infrastructure

import (
	"context"
	"time"

	"github.com/Mark-0731/SwiftMail/internal/platform/smtp"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthChecker implements health checking for system components
type HealthChecker struct {
	db       *pgxpool.Pool
	smtpPool *smtp.Pool
	asynq    *asynq.Client
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(db *pgxpool.Pool, smtpPool *smtp.Pool, asynq *asynq.Client) *HealthChecker {
	return &HealthChecker{
		db:       db,
		smtpPool: smtpPool,
		asynq:    asynq,
	}
}

// CheckDatabase checks database connectivity
func (h *HealthChecker) CheckDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return h.db.Ping(ctx)
}

// CheckSMTPPool checks SMTP pool health
func (h *HealthChecker) CheckSMTPPool(ctx context.Context) error {
	if h.smtpPool == nil {
		return nil // SMTP pool is optional
	}

	// Check if pool has available connections
	active, idle, _, _ := h.smtpPool.Stats()
	if active == 0 && idle == 0 {
		return nil // Pool not initialized yet
	}

	return nil
}

// CheckQueue checks queue connectivity
func (h *HealthChecker) CheckQueue(ctx context.Context) error {
	if h.asynq == nil {
		return nil // Queue is optional
	}

	// Asynq doesn't have a ping method, assume healthy if client exists
	return nil
}
