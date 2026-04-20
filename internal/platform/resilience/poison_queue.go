package resilience

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// PoisonQueueEntry represents a message that has failed repeatedly
type PoisonQueueEntry struct {
	ID                   uuid.UUID       `json:"id"`
	OriginalDLQID        uuid.UUID       `json:"original_dlq_id"`
	TaskType             string          `json:"task_type"`
	Payload              json.RawMessage `json:"payload"`
	FailurePattern       string          `json:"failure_pattern"`
	RepeatedFailureCount int             `json:"repeated_failure_count"`
	FirstFailedAt        time.Time       `json:"first_failed_at"`
	LastFailedAt         time.Time       `json:"last_failed_at"`
	QuarantinedAt        time.Time       `json:"quarantined_at"`
	Reviewed             bool            `json:"reviewed"`
	ReviewNotes          string          `json:"review_notes,omitempty"`
	ReviewedBy           *uuid.UUID      `json:"reviewed_by,omitempty"`
	ReviewedAt           *time.Time      `json:"reviewed_at,omitempty"`
	ResolutionAction     string          `json:"resolution_action,omitempty"` // 'retry', 'discard', 'manual_fix'
	CreatedAt            time.Time       `json:"created_at"`

	// Metadata for analysis
	EmailLogID      *uuid.UUID `json:"email_log_id,omitempty"`
	UserID          *uuid.UUID `json:"user_id,omitempty"`
	RecipientEmail  string     `json:"recipient_email,omitempty"`
	RecipientDomain string     `json:"recipient_domain,omitempty"`
	ErrorCodes      []string   `json:"error_codes,omitempty"`
}

// PoisonQueueFilter for querying poison queue entries
type PoisonQueueFilter struct {
	TaskType        string
	RecipientDomain string
	UserID          *uuid.UUID
	Reviewed        *bool
	FromDate        *time.Time
	ToDate          *time.Time
	MinFailureCount int
	Limit           int
	Offset          int
}

// PoisonQueue manages messages that fail repeatedly
type PoisonQueue struct {
	db                  *pgxpool.Pool
	logger              zerolog.Logger
	failureThreshold    int           // Number of failures before quarantine
	detectionWindow     time.Duration // Time window to detect repeated failures
	autoReviewThreshold int           // Auto-review after this many entries
}

// NewPoisonQueue creates a new poison queue manager
func NewPoisonQueue(db *pgxpool.Pool, logger zerolog.Logger) *PoisonQueue {
	return &PoisonQueue{
		db:                  db,
		logger:              logger,
		failureThreshold:    3, // Quarantine after 3 repeated failures
		detectionWindow:     24 * time.Hour,
		autoReviewThreshold: 100, // Alert when 100+ poison messages
	}
}

// CheckAndQuarantine checks if a DLQ entry should be moved to poison queue
func (pq *PoisonQueue) CheckAndQuarantine(
	ctx context.Context,
	dlqEntryID uuid.UUID,
	emailLogID *uuid.UUID,
) error {
	if emailLogID == nil {
		// Can't track repeated failures without email_log_id
		return nil
	}

	// Count recent failures for this email
	var failureCount int
	var firstFailedAt, lastFailedAt time.Time
	var errorCodes []string

	err := pq.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			MIN(failed_at),
			MAX(failed_at),
			ARRAY_AGG(DISTINCT error_code)
		FROM dead_letter_queue
		WHERE email_log_id = $1
		AND failed_at > NOW() - $2::interval
	`, emailLogID, pq.detectionWindow).Scan(
		&failureCount, &firstFailedAt, &lastFailedAt, &errorCodes,
	)

	if err != nil {
		pq.logger.Error().Err(err).Msg("failed to check repeated failures")
		return err
	}

	// Check if threshold exceeded
	if failureCount < pq.failureThreshold {
		return nil // Not a poison message yet
	}

	// Get DLQ entry details
	var taskType string
	var payload json.RawMessage
	var userID *uuid.UUID
	var recipientEmail, recipientDomain string

	err = pq.db.QueryRow(ctx, `
		SELECT task_type, payload, user_id, recipient_email, recipient_domain
		FROM dead_letter_queue
		WHERE id = $1
	`, dlqEntryID).Scan(&taskType, &payload, &userID, &recipientEmail, &recipientDomain)

	if err != nil {
		return fmt.Errorf("failed to get DLQ entry: %w", err)
	}

	// Analyze failure pattern
	failurePattern := pq.analyzeFailurePattern(errorCodes, failureCount)

	// Create poison queue entry
	entry := &PoisonQueueEntry{
		ID:                   uuid.New(),
		OriginalDLQID:        dlqEntryID,
		TaskType:             taskType,
		Payload:              payload,
		FailurePattern:       failurePattern,
		RepeatedFailureCount: failureCount,
		FirstFailedAt:        firstFailedAt,
		LastFailedAt:         lastFailedAt,
		QuarantinedAt:        time.Now(),
		Reviewed:             false,
		EmailLogID:           emailLogID,
		UserID:               userID,
		RecipientEmail:       recipientEmail,
		RecipientDomain:      recipientDomain,
		ErrorCodes:           errorCodes,
		CreatedAt:            time.Now(),
	}

	// Insert into poison queue
	_, err = pq.db.Exec(ctx, `
		INSERT INTO poison_queue (
			id, original_dlq_id, task_type, payload, failure_pattern,
			repeated_failure_count, first_failed_at, last_failed_at,
			quarantined_at, reviewed, email_log_id, user_id,
			recipient_email, recipient_domain, error_codes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, entry.ID, entry.OriginalDLQID, entry.TaskType, entry.Payload,
		entry.FailurePattern, entry.RepeatedFailureCount, entry.FirstFailedAt,
		entry.LastFailedAt, entry.QuarantinedAt, entry.Reviewed,
		entry.EmailLogID, entry.UserID, entry.RecipientEmail,
		entry.RecipientDomain, entry.ErrorCodes, entry.CreatedAt)

	if err != nil {
		pq.logger.Error().Err(err).Msg("failed to quarantine poison message")
		return fmt.Errorf("failed to quarantine: %w", err)
	}

	pq.logger.Warn().
		Str("poison_id", entry.ID.String()).
		Str("dlq_id", dlqEntryID.String()).
		Str("email_log_id", emailLogID.String()).
		Int("failure_count", failureCount).
		Str("pattern", failurePattern).
		Str("recipient", recipientEmail).
		Msg("message quarantined to poison queue")

	// Check if we should alert
	go pq.checkAlertThreshold(context.Background())

	return nil
}

// analyzeFailurePattern analyzes error codes to determine failure pattern
func (pq *PoisonQueue) analyzeFailurePattern(errorCodes []string, count int) string {
	if len(errorCodes) == 0 {
		return "unknown_pattern"
	}

	// Check for consistent error
	if len(errorCodes) == 1 {
		return fmt.Sprintf("consistent_%s", errorCodes[0])
	}

	// Check for alternating errors
	if len(errorCodes) == 2 {
		return fmt.Sprintf("alternating_%s_%s", errorCodes[0], errorCodes[1])
	}

	// Multiple different errors
	return fmt.Sprintf("varied_errors_%d_types", len(errorCodes))
}

// Get retrieves a poison queue entry by ID
func (pq *PoisonQueue) Get(ctx context.Context, id uuid.UUID) (*PoisonQueueEntry, error) {
	var entry PoisonQueueEntry

	err := pq.db.QueryRow(ctx, `
		SELECT 
			id, original_dlq_id, task_type, payload, failure_pattern,
			repeated_failure_count, first_failed_at, last_failed_at,
			quarantined_at, reviewed, review_notes, reviewed_by, reviewed_at,
			resolution_action, email_log_id, user_id, recipient_email,
			recipient_domain, error_codes, created_at
		FROM poison_queue
		WHERE id = $1
	`, id).Scan(
		&entry.ID, &entry.OriginalDLQID, &entry.TaskType, &entry.Payload,
		&entry.FailurePattern, &entry.RepeatedFailureCount, &entry.FirstFailedAt,
		&entry.LastFailedAt, &entry.QuarantinedAt, &entry.Reviewed,
		&entry.ReviewNotes, &entry.ReviewedBy, &entry.ReviewedAt,
		&entry.ResolutionAction, &entry.EmailLogID, &entry.UserID,
		&entry.RecipientEmail, &entry.RecipientDomain, &entry.ErrorCodes,
		&entry.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("poison queue entry not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get poison queue entry: %w", err)
	}

	return &entry, nil
}

// List retrieves poison queue entries with filtering
func (pq *PoisonQueue) List(
	ctx context.Context,
	filter *PoisonQueueFilter,
) ([]*PoisonQueueEntry, int64, error) {
	if filter == nil {
		filter = &PoisonQueueFilter{Limit: 50, Offset: 0}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	// Build WHERE clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argPos := 1

	if filter.TaskType != "" {
		whereClause += fmt.Sprintf(" AND task_type = $%d", argPos)
		args = append(args, filter.TaskType)
		argPos++
	}
	if filter.RecipientDomain != "" {
		whereClause += fmt.Sprintf(" AND recipient_domain = $%d", argPos)
		args = append(args, filter.RecipientDomain)
		argPos++
	}
	if filter.UserID != nil {
		whereClause += fmt.Sprintf(" AND user_id = $%d", argPos)
		args = append(args, *filter.UserID)
		argPos++
	}
	if filter.Reviewed != nil {
		whereClause += fmt.Sprintf(" AND reviewed = $%d", argPos)
		args = append(args, *filter.Reviewed)
		argPos++
	}
	if filter.FromDate != nil {
		whereClause += fmt.Sprintf(" AND quarantined_at >= $%d", argPos)
		args = append(args, *filter.FromDate)
		argPos++
	}
	if filter.ToDate != nil {
		whereClause += fmt.Sprintf(" AND quarantined_at <= $%d", argPos)
		args = append(args, *filter.ToDate)
		argPos++
	}
	if filter.MinFailureCount > 0 {
		whereClause += fmt.Sprintf(" AND repeated_failure_count >= $%d", argPos)
		args = append(args, filter.MinFailureCount)
		argPos++
	}

	// Get total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM poison_queue %s", whereClause)
	err := pq.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count poison queue entries: %w", err)
	}

	// Get entries
	query := fmt.Sprintf(`
		SELECT 
			id, original_dlq_id, task_type, payload, failure_pattern,
			repeated_failure_count, first_failed_at, last_failed_at,
			quarantined_at, reviewed, review_notes, reviewed_by, reviewed_at,
			resolution_action, email_log_id, user_id, recipient_email,
			recipient_domain, error_codes, created_at
		FROM poison_queue
		%s
		ORDER BY quarantined_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := pq.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query poison queue: %w", err)
	}
	defer rows.Close()

	var entries []*PoisonQueueEntry
	for rows.Next() {
		var entry PoisonQueueEntry

		err := rows.Scan(
			&entry.ID, &entry.OriginalDLQID, &entry.TaskType, &entry.Payload,
			&entry.FailurePattern, &entry.RepeatedFailureCount, &entry.FirstFailedAt,
			&entry.LastFailedAt, &entry.QuarantinedAt, &entry.Reviewed,
			&entry.ReviewNotes, &entry.ReviewedBy, &entry.ReviewedAt,
			&entry.ResolutionAction, &entry.EmailLogID, &entry.UserID,
			&entry.RecipientEmail, &entry.RecipientDomain, &entry.ErrorCodes,
			&entry.CreatedAt,
		)
		if err != nil {
			pq.logger.Error().Err(err).Msg("failed to scan poison queue entry")
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, total, nil
}

// MarkReviewed marks a poison queue entry as reviewed
func (pq *PoisonQueue) MarkReviewed(
	ctx context.Context,
	id uuid.UUID,
	reviewedBy uuid.UUID,
	notes string,
	action string,
) error {
	now := time.Now()

	_, err := pq.db.Exec(ctx, `
		UPDATE poison_queue
		SET reviewed = true,
		    review_notes = $1,
		    reviewed_by = $2,
		    reviewed_at = $3,
		    resolution_action = $4
		WHERE id = $5
	`, notes, reviewedBy, now, action, id)

	if err != nil {
		return fmt.Errorf("failed to mark as reviewed: %w", err)
	}

	pq.logger.Info().
		Str("poison_id", id.String()).
		Str("reviewed_by", reviewedBy.String()).
		Str("action", action).
		Msg("poison queue entry reviewed")

	return nil
}

// Delete removes a poison queue entry
func (pq *PoisonQueue) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := pq.db.Exec(ctx, `DELETE FROM poison_queue WHERE id = $1`, id)
	return err
}

// GetStats returns poison queue statistics
func (pq *PoisonQueue) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total count
	var total, unreviewed int64
	err := pq.db.QueryRow(ctx, `
		SELECT 
			COUNT(*),
			SUM(CASE WHEN reviewed = false THEN 1 ELSE 0 END)
		FROM poison_queue
	`).Scan(&total, &unreviewed)
	if err != nil {
		return nil, err
	}

	stats["total"] = total
	stats["unreviewed"] = unreviewed
	stats["reviewed"] = total - unreviewed

	// Top failure patterns
	rows, err := pq.db.Query(ctx, `
		SELECT failure_pattern, COUNT(*)
		FROM poison_queue
		GROUP BY failure_pattern
		ORDER BY COUNT(*) DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	patterns := make(map[string]int64)
	for rows.Next() {
		var pattern string
		var count int64
		if err := rows.Scan(&pattern, &count); err != nil {
			continue
		}
		patterns[pattern] = count
	}
	stats["top_patterns"] = patterns

	// Top domains
	rows, err = pq.db.Query(ctx, `
		SELECT recipient_domain, COUNT(*)
		FROM poison_queue
		WHERE recipient_domain IS NOT NULL
		GROUP BY recipient_domain
		ORDER BY COUNT(*) DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make(map[string]int64)
	for rows.Next() {
		var domain string
		var count int64
		if err := rows.Scan(&domain, &count); err != nil {
			continue
		}
		domains[domain] = count
	}
	stats["top_domains"] = domains

	return stats, nil
}

// checkAlertThreshold checks if we should alert about poison queue size
func (pq *PoisonQueue) checkAlertThreshold(ctx context.Context) {
	var unreviewed int64
	err := pq.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM poison_queue WHERE reviewed = false
	`).Scan(&unreviewed)

	if err != nil {
		return
	}

	if unreviewed >= int64(pq.autoReviewThreshold) {
		pq.logger.Error().
			Int64("unreviewed_count", unreviewed).
			Int("threshold", pq.autoReviewThreshold).
			Msg("ALERT: Poison queue threshold exceeded - manual review required")

		// TODO: Send alert to on-call engineer (PagerDuty, Slack, etc.)
	}
}

// Cleanup removes old reviewed poison queue entries
func (pq *PoisonQueue) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)

	result, err := pq.db.Exec(ctx, `
		DELETE FROM poison_queue
		WHERE reviewed = true AND reviewed_at < $1
	`, cutoff)

	if err != nil {
		return 0, err
	}

	deleted := result.RowsAffected()
	if deleted > 0 {
		pq.logger.Info().
			Int64("deleted", deleted).
			Time("cutoff", cutoff).
			Msg("poison queue cleanup completed")
	}

	return deleted, nil
}
