package domain

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the domain data access interface.
type Repository interface {
	Create(ctx context.Context, d *Model) error
	GetByID(ctx context.Context, id uuid.UUID) (*Model, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]Model, error)
	GetByDomain(ctx context.Context, userID uuid.UUID, domain string) (*Model, error)
	Update(ctx context.Context, d *Model) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

// PostgresRepository implements Repository.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new domain repository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, d *Model) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO domains (user_id, domain, status, spf_record, dkim_public_key, dkim_private_key_encrypted, dkim_selector, dmarc_record, bimi_logo_url, bimi_vmc_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, created_at`,
		d.UserID, d.Domain, d.Status, d.SPFRecord, d.DKIMPublicKey, d.DKIMPrivateKeyEncrypted,
		d.DKIMSelector, d.DMARCRecord, d.BIMILogoURL, d.BIMIVmcURL,
	).Scan(&d.ID, &d.CreatedAt)
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Model, error) {
	d := &Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, domain, status, spf_record, dkim_public_key, dkim_private_key_encrypted, dkim_selector, dmarc_record,
		        bimi_logo_url, bimi_vmc_url, mx_verified, warmup_day, warmup_active, last_verified_at, dkim_rotated_at, created_at
		 FROM domains WHERE id = $1`, id,
	).Scan(&d.ID, &d.UserID, &d.Domain, &d.Status, &d.SPFRecord, &d.DKIMPublicKey, &d.DKIMPrivateKeyEncrypted,
		&d.DKIMSelector, &d.DMARCRecord, &d.BIMILogoURL, &d.BIMIVmcURL, &d.MXVerified, &d.WarmupDay,
		&d.WarmupActive, &d.LastVerifiedAt, &d.DKIMRotatedAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]Model, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, domain, status, spf_record, dkim_public_key, dkim_selector, dmarc_record,
		        bimi_logo_url, bimi_vmc_url, mx_verified, warmup_day, warmup_active, last_verified_at, dkim_rotated_at, created_at
		 FROM domains WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []Model
	for rows.Next() {
		d := Model{}
		if err := rows.Scan(&d.ID, &d.UserID, &d.Domain, &d.Status, &d.SPFRecord, &d.DKIMPublicKey,
			&d.DKIMSelector, &d.DMARCRecord, &d.BIMILogoURL, &d.BIMIVmcURL, &d.MXVerified,
			&d.WarmupDay, &d.WarmupActive, &d.LastVerifiedAt, &d.DKIMRotatedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

func (r *PostgresRepository) GetByDomain(ctx context.Context, userID uuid.UUID, domain string) (*Model, error) {
	d := &Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, domain, status, spf_record, dkim_public_key, dkim_private_key_encrypted, dkim_selector, dmarc_record,
		        mx_verified, warmup_day, warmup_active, last_verified_at, created_at
		 FROM domains WHERE user_id = $1 AND domain = $2`, userID, domain,
	).Scan(&d.ID, &d.UserID, &d.Domain, &d.Status, &d.SPFRecord, &d.DKIMPublicKey, &d.DKIMPrivateKeyEncrypted,
		&d.DKIMSelector, &d.DMARCRecord, &d.MXVerified, &d.WarmupDay, &d.WarmupActive, &d.LastVerifiedAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *PostgresRepository) Update(ctx context.Context, d *Model) error {
	_, err := r.db.Exec(ctx,
		`UPDATE domains SET status=$1, spf_record=$2, dkim_public_key=$3, dkim_private_key_encrypted=$4,
		        dkim_selector=$5, dmarc_record=$6, mx_verified=$7, last_verified_at=$8, dkim_rotated_at=$9
		 WHERE id=$10`,
		d.Status, d.SPFRecord, d.DKIMPublicKey, d.DKIMPrivateKeyEncrypted, d.DKIMSelector,
		d.DMARCRecord, d.MXVerified, d.LastVerifiedAt, d.DKIMRotatedAt, d.ID,
	)
	return err
}

func (r *PostgresRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM domains WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}
