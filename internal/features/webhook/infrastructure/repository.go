package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	webhook "github.com/Mark-0731/SwiftMail/internal/features/webhook"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository manages webhook configurations in PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a webhook repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, w *webhook.Config) error {
	// Generate a signing secret
	secretBytes := make([]byte, 32)
	rand.Read(secretBytes)
	w.Secret = hex.EncodeToString(secretBytes)

	return r.db.QueryRow(ctx,
		`INSERT INTO webhooks (user_id, url, secret, events, active) VALUES ($1, $2, $3, $4, TRUE) RETURNING id, created_at`,
		w.UserID, w.URL, w.Secret, w.Events,
	).Scan(&w.ID, &w.CreatedAt)
}

func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*webhook.Config, error) {
	w := &webhook.Config{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, url, secret, events, active, created_at FROM webhooks WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&w.ID, &w.UserID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt)
	return w, err
}

func (r *Repository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]webhook.Config, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, url, secret, events, active, created_at FROM webhooks WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []webhook.Config
	for rows.Next() {
		w := webhook.Config{}
		if err := rows.Scan(&w.ID, &w.UserID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, w)
	}
	return webhooks, nil
}

func (r *Repository) ToggleActive(ctx context.Context, id, userID uuid.UUID, active bool) error {
	_, err := r.db.Exec(ctx, `UPDATE webhooks SET active = $1 WHERE id = $2 AND user_id = $3`, active, id, userID)
	return err
}

func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}
