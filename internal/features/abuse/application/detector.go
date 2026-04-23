package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/pkg/database"
)

// Detector monitors bounce and complaint rates to auto-block abusive users.
type Detector struct {
	db     database.Querier
	rdb    *redis.Client
	logger zerolog.Logger

	BounceRateThreshold    float64 // 5%
	ComplaintRateThreshold float64 // 0.1%
	SpikeMultiplier        float64 // 3x
	EvalWindow             time.Duration
}

// NewDetector creates a new abuse detector.
func NewDetector(db database.Querier, rdb *redis.Client, logger zerolog.Logger) *Detector {
	return &Detector{
		db:                     db,
		rdb:                    rdb,
		logger:                 logger,
		BounceRateThreshold:    5.0,
		ComplaintRateThreshold: 0.1,
		SpikeMultiplier:        3.0,
		EvalWindow:             1 * time.Hour,
	}
}

// CheckUser evaluates a user's sending behavior and takes graduated action.
func (d *Detector) CheckUser(ctx context.Context, userID uuid.UUID) (action string, err error) {
	// Get counts from last hour
	var sent, bounced, complained int64

	window := time.Now().Add(-d.EvalWindow)

	d.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_logs WHERE user_id = $1 AND created_at >= $2`, userID, window,
	).Scan(&sent)

	d.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_logs WHERE user_id = $1 AND status = 'bounced' AND updated_at >= $2`, userID, window,
	).Scan(&bounced)

	d.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_logs WHERE user_id = $1 AND status = 'complained' AND updated_at >= $2`, userID, window,
	).Scan(&complained)

	if sent == 0 {
		return "none", nil
	}

	bounceRate := float64(bounced) / float64(sent) * 100
	complaintRate := float64(complained) / float64(sent) * 100

	// Hard block on high bounce rate
	if bounceRate > d.BounceRateThreshold {
		d.logger.Warn().
			Str("user_id", userID.String()).
			Float64("bounce_rate", bounceRate).
			Msg("high bounce rate detected — suspending user")

		d.suspendUser(ctx, userID, fmt.Sprintf("bounce rate %.2f%% exceeds threshold %.2f%%", bounceRate, d.BounceRateThreshold))
		return "suspended", nil
	}

	// Hard block on high complaint rate
	if complaintRate > d.ComplaintRateThreshold {
		d.logger.Warn().
			Str("user_id", userID.String()).
			Float64("complaint_rate", complaintRate).
			Msg("high complaint rate detected — suspending user")

		d.suspendUser(ctx, userID, fmt.Sprintf("complaint rate %.2f%% exceeds threshold %.2f%%", complaintRate, d.ComplaintRateThreshold))
		return "suspended", nil
	}

	// Warn on elevated rates
	if bounceRate > d.BounceRateThreshold/2 || complaintRate > d.ComplaintRateThreshold/2 {
		d.warnUser(ctx, userID)
		return "warned", nil
	}

	return "none", nil
}

func (d *Detector) suspendUser(ctx context.Context, userID uuid.UUID, reason string) {
	d.db.Exec(ctx, `UPDATE users SET status = 'suspended', updated_at = NOW() WHERE id = $1`, userID)

	d.db.Exec(ctx,
		`INSERT INTO audit_logs (user_id, action, resource_type, resource_id, metadata)
		 VALUES ($1, 'auto_suspend', 'user', $2, $3)`,
		userID, userID, fmt.Sprintf(`{"reason":"%s"}`, reason),
	)
}

func (d *Detector) warnUser(ctx context.Context, userID uuid.UUID) {
	d.db.Exec(ctx, `UPDATE users SET status = 'warned', updated_at = NOW() WHERE id = $1 AND status = 'active'`, userID)
}
