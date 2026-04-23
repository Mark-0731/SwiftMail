package infrastructure

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Mark-0731/SwiftMail/internal/features/verification"
	"github.com/Mark-0731/SwiftMail/pkg/database"
)

// PostgresRepository implements verification.Repository
type PostgresRepository struct {
	db database.Querier
}

// NewPostgresRepository creates a new PostgreSQL verification repository
func NewPostgresRepository(db database.Querier) verification.Repository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateVerificationToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (r *PostgresRepository) GetVerificationToken(ctx context.Context, tokenHash string) (*verification.VerificationToken, error) {
	token := &verification.VerificationToken{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at, created_at
		 FROM email_verification_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.UsedAt, &token.CreatedAt)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (r *PostgresRepository) MarkTokenUsed(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = NOW() WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *PostgresRepository) DeleteToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM email_verification_tokens WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *PostgresRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *PostgresRepository) GetUserEmail(ctx context.Context, userID uuid.UUID) (email string, name string, verified bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT email, name, email_verified FROM users WHERE id = $1`,
		userID,
	).Scan(&email, &name, &verified)
	return
}
