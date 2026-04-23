package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/pkg/database"
)

// DLQEntry represents a failed task in the dead letter queue
type DLQEntry struct {
	ID              uuid.UUID       `json:"id"`
	TaskType        string          `json:"task_type"`
	TaskID          string          `json:"task_id,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	FailureReason   string          `json:"failure_reason"`
	ErrorCode       string          `json:"error_code"`
	SMTPResponse    string          `json:"smtp_response,omitempty"`
	RetryCount      int             `json:"retry_count"`
	MaxRetries      int             `json:"max_retries"`
	EmailLogID      *uuid.UUID      `json:"email_log_id,omitempty"`
	UserID          *uuid.UUID      `json:"user_id,omitempty"`
	RecipientEmail  string          `json:"recipient_email,omitempty"`
	RecipientDomain string          `json:"recipient_domain,omitempty"`
	FailedAt        time.Time       `json:"failed_at"`
	CreatedAt       time.Time       `json:"created_at"`
	RetriedAt       *time.Time      `json:"retried_at,omitempty"`
	RetryStatus     string          `json:"retry_status,omitempty"`
}

// DLQFilter for querying DLQ entries
type DLQFilter struct {
	TaskType        string
	ErrorCode       string
	RecipientDomain string
	UserID          *uuid.UUID
	RetryStatus     string
	FromDate        *time.Time
	ToDate          *time.Time
	Limit           int
	Offset          int
}

// DeadLetterQueue manages permanently failed tasks
type DeadLetterQueue struct {
	db        database.Querier
	logger    zerolog.Logger
	retention time.Duration // How long to keep DLQ entries (default: 30 days)
}

// NewDeadLetterQueue creates a new DLQ manager
func NewDeadLetterQueue(db database.Querier, retention time.Duration, logger zerolog.Logger) *DeadLetterQueue {
	if retention == 0 {
		retention = 30 * 24 * time.Hour // 30 days default
	}

	return &DeadLetterQueue{
		db:        db,
		logger:    logger,
		retention: retention,
	}
}

// Add moves a failed task to the DLQ
func (dlq *DeadLetterQueue) Add(ctx context.Context, entry *DLQEntry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.FailedAt.IsZero() {
		entry.FailedAt = time.Now()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.MaxRetries == 0 {
		entry.MaxRetries = 3
	}

	_, err := dlq.db.Exec(ctx, `
		INSERT INTO dead_letter_queue (
			id, task_type, task_id, payload, failure_reason, error_code, smtp_response,
			retry_count, max_retries, email_log_id, user_id, recipient_email, recipient_domain,
			failed_at, created_at, retry_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, entry.ID, entry.TaskType, entry.TaskID, entry.Payload, entry.FailureReason,
		entry.ErrorCode, entry.SMTPResponse, entry.RetryCount, entry.MaxRetries,
		entry.EmailLogID, entry.UserID, entry.RecipientEmail, entry.RecipientDomain,
		entry.FailedAt, entry.CreatedAt, "pending")

	if err != nil {
		dlq.logger.Error().
			Err(err).
			Str("task_type", entry.TaskType).
			Str("error_code", entry.ErrorCode).
			Msg("failed to add task to DLQ")
		return fmt.Errorf("failed to add to DLQ: %w", err)
	}

	dlq.logger.Warn().
		Str("dlq_id", entry.ID.String()).
		Str("task_type", entry.TaskType).
		Str("error_code", entry.ErrorCode).
		Int("retry_count", entry.RetryCount).
		Str("recipient", entry.RecipientEmail).
		Msg("task moved to DLQ")

	return nil
}

// Get retrieves a DLQ entry by ID
func (dlq *DeadLetterQueue) Get(ctx context.Context, id uuid.UUID) (*DLQEntry, error) {
	var entry DLQEntry

	err := dlq.db.QueryRow(ctx, `
		SELECT id, task_type, task_id, payload, failure_reason, error_code, smtp_response,
		       retry_count, max_retries, email_log_id, user_id, recipient_email, recipient_domain,
		       failed_at, created_at, retried_at, retry_status
		FROM dead_letter_queue
		WHERE id = $1
	`, id).Scan(
		&entry.ID, &entry.TaskType, &entry.TaskID, &entry.Payload, &entry.FailureReason,
		&entry.ErrorCode, &entry.SMTPResponse, &entry.RetryCount, &entry.MaxRetries,
		&entry.EmailLogID, &entry.UserID, &entry.RecipientEmail, &entry.RecipientDomain,
		&entry.FailedAt, &entry.CreatedAt, &entry.RetriedAt, &entry.RetryStatus,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("DLQ entry not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get DLQ entry: %w", err)
	}

	return &entry, nil
}

// List retrieves DLQ entries with filtering and pagination
func (dlq *DeadLetterQueue) List(ctx context.Context, filter *DLQFilter) ([]*DLQEntry, int64, error) {
	if filter == nil {
		filter = &DLQFilter{Limit: 50, Offset: 0}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000 // Max limit
	}

	// Build WHERE clause dynamically
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argPos := 1

	if filter.TaskType != "" {
		whereClause += fmt.Sprintf(" AND task_type = $%d", argPos)
		args = append(args, filter.TaskType)
		argPos++
	}
	if filter.ErrorCode != "" {
		whereClause += fmt.Sprintf(" AND error_code = $%d", argPos)
		args = append(args, filter.ErrorCode)
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
	if filter.RetryStatus != "" {
		whereClause += fmt.Sprintf(" AND retry_status = $%d", argPos)
		args = append(args, filter.RetryStatus)
		argPos++
	}
	if filter.FromDate != nil {
		whereClause += fmt.Sprintf(" AND failed_at >= $%d", argPos)
		args = append(args, *filter.FromDate)
		argPos++
	}
	if filter.ToDate != nil {
		whereClause += fmt.Sprintf(" AND failed_at <= $%d", argPos)
		args = append(args, *filter.ToDate)
		argPos++
	}

	// Get total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM dead_letter_queue %s", whereClause)
	err := dlq.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count DLQ entries: %w", err)
	}

	// Get entries with pagination
	query := fmt.Sprintf(`
		SELECT id, task_type, task_id, payload, failure_reason, error_code, smtp_response,
		       retry_count, max_retries, email_log_id, user_id, recipient_email, recipient_domain,
		       failed_at, created_at, retried_at, retry_status
		FROM dead_letter_queue
		%s
		ORDER BY failed_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := dlq.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query DLQ entries: %w", err)
	}
	defer rows.Close()

	var entries []*DLQEntry
	for rows.Next() {
		var entry DLQEntry

		err := rows.Scan(
			&entry.ID, &entry.TaskType, &entry.TaskID, &entry.Payload, &entry.FailureReason,
			&entry.ErrorCode, &entry.SMTPResponse, &entry.RetryCount, &entry.MaxRetries,
			&entry.EmailLogID, &entry.UserID, &entry.RecipientEmail, &entry.RecipientDomain,
			&entry.FailedAt, &entry.CreatedAt, &entry.RetriedAt, &entry.RetryStatus,
		)
		if err != nil {
			dlq.logger.Error().Err(err).Msg("failed to scan DLQ entry")
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, total, nil
}

// Retry moves a DLQ entry back to the main queue
func (dlq *DeadLetterQueue) Retry(ctx context.Context, id uuid.UUID, queue Queue) error {
	entry, err := dlq.Get(ctx, id)
	if err != nil {
		return err
	}

	// Check if already retried
	if entry.RetryStatus == "retrying" || entry.RetryStatus == "success" {
		return fmt.Errorf("DLQ entry already retried (status: %s)", entry.RetryStatus)
	}

	// Mark as retrying
	now := time.Now()
	_, err = dlq.db.Exec(ctx, `
		UPDATE dead_letter_queue
		SET retry_status = 'retrying', retried_at = $1
		WHERE id = $2
	`, now, id)
	if err != nil {
		return fmt.Errorf("failed to mark DLQ entry as retrying: %w", err)
	}

	// Re-enqueue the task
	task := &Task{
		Type:    entry.TaskType,
		Payload: entry.Payload,
	}

	opts := &EnqueueOptions{
		Queue:    "default",
		MaxRetry: 3,
		TaskID:   fmt.Sprintf("dlq-retry:%s", id.String()),
	}

	if err := queue.EnqueueWithOptions(ctx, task, opts); err != nil {
		// Revert status on failure
		dlq.db.Exec(ctx, `
			UPDATE dead_letter_queue
			SET retry_status = 'failed_again'
			WHERE id = $1
		`, id)

		dlq.logger.Error().
			Err(err).
			Str("dlq_id", id.String()).
			Msg("failed to retry DLQ task")
		return fmt.Errorf("failed to re-enqueue task: %w", err)
	}

	// Mark as success (will be deleted after confirmation)
	_, err = dlq.db.Exec(ctx, `
		UPDATE dead_letter_queue
		SET retry_status = 'success'
		WHERE id = $1
	`, id)
	if err != nil {
		dlq.logger.Warn().Err(err).Msg("failed to update retry status")
	}

	dlq.logger.Info().
		Str("dlq_id", id.String()).
		Str("task_type", entry.TaskType).
		Msg("DLQ task retried successfully")

	return nil
}

// RetryBatch retries multiple DLQ entries
func (dlq *DeadLetterQueue) RetryBatch(ctx context.Context, ids []uuid.UUID, queue Queue) (int, []error) {
	successCount := 0
	var errors []error

	for _, id := range ids {
		if err := dlq.Retry(ctx, id, queue); err != nil {
			errors = append(errors, fmt.Errorf("failed to retry %s: %w", id, err))
		} else {
			successCount++
		}
	}

	return successCount, errors
}

// Delete removes a DLQ entry
func (dlq *DeadLetterQueue) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := dlq.db.Exec(ctx, `DELETE FROM dead_letter_queue WHERE id = $1`, id)
	return err
}

// GetStats returns DLQ statistics
func (dlq *DeadLetterQueue) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total count
	var total int64
	err := dlq.db.QueryRow(ctx, `SELECT COUNT(*) FROM dead_letter_queue`).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// Count by status
	rows, err := dlq.db.Query(ctx, `
		SELECT retry_status, COUNT(*) 
		FROM dead_letter_queue 
		GROUP BY retry_status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statusCounts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		statusCounts[status] = count
	}
	stats["by_status"] = statusCounts

	// Count by error code
	rows, err = dlq.db.Query(ctx, `
		SELECT error_code, COUNT(*) 
		FROM dead_letter_queue 
		WHERE error_code IS NOT NULL
		GROUP BY error_code 
		ORDER BY COUNT(*) DESC 
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	errorCounts := make(map[string]int64)
	for rows.Next() {
		var errorCode string
		var count int64
		if err := rows.Scan(&errorCode, &count); err != nil {
			continue
		}
		errorCounts[errorCode] = count
	}
	stats["top_errors"] = errorCounts

	// Count by domain
	rows, err = dlq.db.Query(ctx, `
		SELECT recipient_domain, COUNT(*) 
		FROM dead_letter_queue 
		WHERE recipient_domain IS NOT NULL
		GROUP BY recipient_domain 
		ORDER BY COUNT(*) DESC 
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domainCounts := make(map[string]int64)
	for rows.Next() {
		var domain string
		var count int64
		if err := rows.Scan(&domain, &count); err != nil {
			continue
		}
		domainCounts[domain] = count
	}
	stats["top_domains"] = domainCounts

	return stats, nil
}

// Cleanup removes old DLQ entries based on retention policy
func (dlq *DeadLetterQueue) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-dlq.retention)

	// Only delete successfully retried entries older than retention
	result, err := dlq.db.Exec(ctx, `
		DELETE FROM dead_letter_queue
		WHERE created_at < $1 AND retry_status = 'success'
	`, cutoff)

	if err != nil {
		dlq.logger.Error().Err(err).Msg("failed to cleanup DLQ")
		return 0, err
	}

	deleted := result.RowsAffected()
	if deleted > 0 {
		dlq.logger.Info().
			Int64("deleted", deleted).
			Time("cutoff", cutoff).
			Msg("DLQ cleanup completed")
	}

	return deleted, nil
}

// StartCleanupWorker starts a background worker to cleanup old DLQ entries
func (dlq *DeadLetterQueue) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = 24 * time.Hour // Daily cleanup by default
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	dlq.logger.Info().
		Dur("interval", interval).
		Dur("retention", dlq.retention).
		Msg("DLQ cleanup worker started")

	for {
		select {
		case <-ticker.C:
			if _, err := dlq.Cleanup(ctx); err != nil {
				dlq.logger.Error().Err(err).Msg("DLQ cleanup failed")
			}
		case <-ctx.Done():
			dlq.logger.Info().Msg("DLQ cleanup worker stopped")
			return
		}
	}
}
