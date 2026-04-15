package suppression

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the suppression data access interface.
type Repository interface {
	Add(ctx context.Context, userID uuid.UUID, email, suppressionType, reason string) error
	AddGlobal(ctx context.Context, email, suppressionType, reason string) error
	Remove(ctx context.Context, id uuid.UUID, userID uuid.UUID) (string, error)
	List(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Entry, int64, error)
}

// PostgresRepository implements Repository.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new suppression repository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Add(ctx context.Context, userID uuid.UUID, email, suppressionType, reason string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO suppression_list (user_id, email, type, reason) VALUES ($1, $2, $3, $4) ON CONFLICT (user_id, email) DO NOTHING`,
		userID, email, suppressionType, reason,
	)
	if err != nil {
		return fmt.Errorf("failed to add to suppression list: %w", err)
	}
	return nil
}

func (r *PostgresRepository) AddGlobal(ctx context.Context, email, suppressionType, reason string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO suppression_list (user_id, email, type, reason) VALUES (NULL, $1, $2, $3) ON CONFLICT DO NOTHING`,
		email, suppressionType, reason,
	)
	return err
}

func (r *PostgresRepository) Remove(ctx context.Context, id uuid.UUID, userID uuid.UUID) (string, error) {
	var email string
	err := r.db.QueryRow(ctx, `SELECT email FROM suppression_list WHERE id = $1 AND user_id = $2`, id, userID).Scan(&email)
	if err != nil {
		return "", fmt.Errorf("suppression entry not found: %w", err)
	}

	_, err = r.db.Exec(ctx, `DELETE FROM suppression_list WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return "", err
	}

	return email, nil
}

func (r *PostgresRepository) List(ctx context.Context, userID uuid.UUID, page, perPage int) ([]Entry, int64, error) {
	var total int64
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM suppression_list WHERE user_id = $1`, userID).Scan(&total)

	if perPage <= 0 {
		perPage = 50
	}
	offset := (page - 1) * perPage

	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, email, type, reason, created_at FROM suppression_list WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, perPage, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		e := Entry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.Email, &e.Type, &e.Reason, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}
