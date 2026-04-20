package infrastructure

import (
	"context"
	"fmt"

	"github.com/Mark-0731/SwiftMail/internal/features/user"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines user data access.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a user repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetByID returns a user by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u := &user.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, name, role, status, totp_enabled, email_verified, created_at, updated_at, suspended_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Status, &u.TOTPEnabled, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt, &u.SuspendedAt)
	return u, err
}

// GetByEmail returns a user by email.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	u := &user.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, name, password_hash, role, status, totp_enabled, totp_secret, email_verified, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.Status, &u.TOTPEnabled, &u.TOTPSecret, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// UpdateProfile updates name.
func (r *Repository) UpdateProfile(ctx context.Context, id uuid.UUID, name string) error {
	result, err := r.db.Exec(ctx, `UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2`, name, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// ListAll returns all users (admin).
func (r *Repository) ListAll(ctx context.Context, page, perPage int) ([]user.User, int64, error) {
	var total int64
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)

	offset := (page - 1) * perPage
	rows, err := r.db.Query(ctx,
		`SELECT id, email, name, role, status, totp_enabled, email_verified, created_at, suspended_at
		 FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, perPage, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []user.User
	for rows.Next() {
		u := user.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Status, &u.TOTPEnabled, &u.EmailVerified, &u.CreatedAt, &u.SuspendedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, nil
}

// Suspend suspends a user.
func (r *Repository) Suspend(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET status = 'suspended', suspended_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

// Unsuspend reactivates a user.
func (r *Repository) Unsuspend(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET status = 'active', suspended_at = NULL, updated_at = NOW() WHERE id = $1`, id)
	return err
}
