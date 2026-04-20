package infrastructure

import (
emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ensure PostgresEmailRepository implements EmailRepository
var _ EmailRepository = (*PostgresEmailRepository)(nil)

// PostgresEmailRepository implements EmailRepository using PostgreSQL.
type PostgresEmailRepository struct {
	db *pgxpool.Pool
}

// NewPostgresEmailRepository creates a new PostgreSQL email repository.
func NewPostgresEmailRepository(db *pgxpool.Pool) EmailRepository {
	return &PostgresEmailRepository{db: db}
}

func (r *PostgresEmailRepository) Create(ctx context.Context, e *emailtypes.Model) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO email_logs (user_id, domain_id, idempotency_key, message_id, from_email, to_email, subject, status, template_id, tags, attachments, metadata, max_retries)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id, created_at, updated_at, status_changed_at`,
		e.UserID, e.DomainID, e.IdempotencyKey, e.MessageID, e.FromEmail, e.ToEmail,
		e.Subject, e.Status, e.TemplateID, e.Tags, e.Attachments, e.Metadata, e.MaxRetries,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.StatusChangedAt)
}

func (r *PostgresEmailRepository) GetByID(ctx context.Context, id uuid.UUID) (*emailtypes.Model, error) {
	e := &emailtypes.Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, domain_id, idempotency_key, message_id, from_email, to_email, subject,
		        status, previous_status, status_changed_at, template_id, tags, ip_used, smtp_response,
		        retry_count, max_retries, attachments, metadata, opened_at, clicked_at, bounced_at, created_at, updated_at
		 FROM email_logs WHERE id = $1`, id,
	).Scan(
		&e.ID, &e.UserID, &e.DomainID, &e.IdempotencyKey, &e.MessageID, &e.FromEmail, &e.ToEmail,
		&e.Subject, &e.Status, &e.PreviousStatus, &e.StatusChangedAt, &e.TemplateID, &e.Tags,
		&e.IPUsed, &e.SMTPResponse, &e.RetryCount, &e.MaxRetries, &e.Attachments, &e.Metadata,
		&e.OpenedAt, &e.ClickedAt, &e.BouncedAt, &e.CreatedAt, &e.UpdatedAt,
	)
	return e, err
}

func (r *PostgresEmailRepository) UpdateStatus(ctx context.Context, id uuid.UUID, from, to string, smtpResponse *string) error {
	result, err := r.db.Exec(ctx,
		`UPDATE email_logs SET previous_status = status, status = $1, status_changed_at = NOW(), updated_at = NOW(), smtp_response = COALESCE($2, smtp_response)
		 WHERE id = $3 AND status = $4`,
		to, smtpResponse, id, from,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("status transition from %s to %s failed: current status mismatch", from, to)
	}
	return nil
}

func (r *PostgresEmailRepository) Search(ctx context.Context, q *emailtypes.LogQuery) ([]emailtypes.Model, int64, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, q.UserID)
	argIdx++

	if q.Email != "" {
		conditions = append(conditions, fmt.Sprintf("(to_email ILIKE $%d OR from_email ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q.Email+"%")
		argIdx++
	}
	if q.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Tag != "" {
		conditions = append(conditions, fmt.Sprintf("tags @> $%d::jsonb", argIdx))
		args = append(args, fmt.Sprintf(`["%s"]`, q.Tag))
		argIdx++
	}
	if q.DateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, q.DateFrom)
		argIdx++
	}
	if q.DateTo != "" {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, q.DateTo)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM email_logs WHERE %s", where)
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get page
	if q.PerPage <= 0 {
		q.PerPage = 50
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PerPage

	query := fmt.Sprintf(
		`SELECT id, user_id, domain_id, message_id, from_email, to_email, subject, status, tags,
		        ip_used, smtp_response, retry_count, attachments, opened_at, clicked_at, bounced_at, created_at
		 FROM email_logs WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, q.PerPage, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []emailtypes.Model
	for rows.Next() {
		e := emailtypes.Model{}
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.DomainID, &e.MessageID, &e.FromEmail, &e.ToEmail,
			&e.Subject, &e.Status, &e.Tags, &e.IPUsed, &e.SMTPResponse, &e.RetryCount,
			&e.Attachments, &e.OpenedAt, &e.ClickedAt, &e.BouncedAt, &e.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		results = append(results, e)
	}

	return results, total, nil
}

func (r *PostgresEmailRepository) IncrementRetry(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_logs SET retry_count = retry_count + 1, updated_at = NOW() WHERE id = $1`, id,
	)
	return err
}

func (r *PostgresEmailRepository) SetOpened(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_logs SET opened_at = COALESCE(opened_at, NOW()), updated_at = NOW() WHERE id = $1`, id,
	)
	return err
}

func (r *PostgresEmailRepository) SetClicked(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_logs SET clicked_at = COALESCE(clicked_at, NOW()), updated_at = NOW() WHERE id = $1`, id,
	)
	return err
}
