package infrastructure

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the interface for auth-related database operations.
type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash, name string) (*UserModel, error)
	GetUserByEmail(ctx context.Context, email string) (*UserModel, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*UserModel, error)
	UpdateTOTPSecret(ctx context.Context, userID uuid.UUID, secret string) error
	EnableTOTP(ctx context.Context, userID uuid.UUID) error
	DisableTOTP(ctx context.Context, userID uuid.UUID) error
	CreateAPIKey(ctx context.Context, userID uuid.UUID, name, keyHash, keyPrefix string, permissions []string, expiresAt *time.Time) (*APIKeyModel, error)
	GetAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]APIKeyModel, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyModel, error)
	DeleteAPIKey(ctx context.Context, id, userID uuid.UUID) error
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error

	// Password reset
	CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetPasswordResetToken(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) error
	DeletePasswordResetToken(ctx context.Context, tokenHash string) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error

	// Email verification
	CreateEmailVerificationToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetEmailVerificationToken(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)
	MarkEmailVerificationTokenUsed(ctx context.Context, tokenHash string) error
	DeleteEmailVerificationToken(ctx context.Context, tokenHash string) error
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error

	// Session management
	IncrementTokenVersion(ctx context.Context, userID uuid.UUID) error
}

// UserModel represents a user record in the database.
type UserModel struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	TOTPSecret    *string   `json:"-"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	EmailVerified bool      `json:"email_verified"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// APIKeyModel represents an API key record in the database.
type APIKeyModel struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	KeyHash     string     `json:"-"`
	KeyPrefix   string     `json:"key_prefix"`
	Permissions []string   `json:"permissions"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// PostgresRepository implements Repository with PostgreSQL.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgreSQL-backed auth repository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, email, passwordHash, name string) (*UserModel, error) {
	user := &UserModel{}
	err := r.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, name, role, totp_secret, totp_enabled, email_verified, status, created_at, updated_at`,
		email, passwordHash, name,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&user.TOTPSecret, &user.TOTPEnabled, &user.EmailVerified, &user.Status,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*UserModel, error) {
	user := &UserModel{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, totp_secret, totp_enabled, email_verified, status, created_at, updated_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&user.TOTPSecret, &user.TOTPEnabled, &user.EmailVerified, &user.Status,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*UserModel, error) {
	user := &UserModel{}
	err := r.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, totp_secret, totp_enabled, email_verified, status, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&user.TOTPSecret, &user.TOTPEnabled, &user.EmailVerified, &user.Status,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepository) UpdateTOTPSecret(ctx context.Context, userID uuid.UUID, secret string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET totp_secret = $1, updated_at = NOW() WHERE id = $2`,
		secret, userID,
	)
	return err
}

func (r *PostgresRepository) EnableTOTP(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET totp_enabled = TRUE, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *PostgresRepository) DisableTOTP(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET totp_enabled = FALSE, totp_secret = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *PostgresRepository) CreateAPIKey(ctx context.Context, userID uuid.UUID, name, keyHash, keyPrefix string, permissions []string, expiresAt *time.Time) (*APIKeyModel, error) {
	key := &APIKeyModel{}
	err := r.db.QueryRow(ctx,
		`INSERT INTO api_keys (user_id, name, key_hash, key_prefix, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, user_id, name, key_hash, key_prefix, last_used_at, expires_at, created_at`,
		userID, name, keyHash, keyPrefix, permissions, expiresAt,
	).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (r *PostgresRepository) GetAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]APIKeyModel, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, name, key_hash, key_prefix, last_used_at, expires_at, created_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyModel
	for rows.Next() {
		var k APIKeyModel
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (r *PostgresRepository) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyModel, error) {
	key := &APIKeyModel{}
	err := r.db.QueryRow(ctx,
		`SELECT ak.id, ak.user_id, ak.name, ak.key_hash, ak.key_prefix, ak.last_used_at, ak.expires_at, ak.created_at
		 FROM api_keys ak
		 JOIN users u ON ak.user_id = u.id
		 WHERE ak.key_hash = $1`,
		keyHash,
	).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (r *PostgresRepository) DeleteAPIKey(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM api_keys WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

func (r *PostgresRepository) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

// PasswordResetToken represents a password reset token.
type PasswordResetToken struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// EmailVerificationToken represents an email verification token.
type EmailVerificationToken struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (r *PostgresRepository) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (r *PostgresRepository) GetPasswordResetToken(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	token := &PasswordResetToken{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.UsedAt, &token.CreatedAt)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (r *PostgresRepository) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = NOW() WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *PostgresRepository) DeletePasswordResetToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM password_reset_tokens WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *PostgresRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		passwordHash, userID,
	)
	return err
}

func (r *PostgresRepository) CreateEmailVerificationToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (r *PostgresRepository) GetEmailVerificationToken(ctx context.Context, tokenHash string) (*EmailVerificationToken, error) {
	token := &EmailVerificationToken{}
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

func (r *PostgresRepository) MarkEmailVerificationTokenUsed(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = NOW() WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *PostgresRepository) DeleteEmailVerificationToken(ctx context.Context, tokenHash string) error {
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

func (r *PostgresRepository) IncrementTokenVersion(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET token_version = COALESCE(token_version, 0) + 1, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}
