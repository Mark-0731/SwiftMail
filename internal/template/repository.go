package template

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the template data access interface.
type Repository interface {
	Create(ctx context.Context, t *Model) error
	GetByID(ctx context.Context, id uuid.UUID) (*Model, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]Model, error)
	Update(ctx context.Context, t *Model) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	CreateVersion(ctx context.Context, v *Version) error
	GetVersions(ctx context.Context, templateID uuid.UUID) ([]Version, error)
	GetActiveVersion(ctx context.Context, templateID uuid.UUID) (*Version, error)
	GetVersion(ctx context.Context, templateID uuid.UUID, version int) (*Version, error)
}

// PostgresRepository implements Repository.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new template repository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t *Model) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO templates (user_id, name, description) VALUES ($1, $2, $3) RETURNING id, active_version, created_at, updated_at`,
		t.UserID, t.Name, t.Description,
	).Scan(&t.ID, &t.ActiveVersion, &t.CreatedAt, &t.UpdatedAt)
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Model, error) {
	t := &Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, name, description, active_version, archived, created_at, updated_at FROM templates WHERE id = $1`, id,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.ActiveVersion, &t.Archived, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]Model, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, name, description, active_version, archived, created_at, updated_at
		 FROM templates WHERE user_id = $1 AND archived = FALSE ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []Model
	for rows.Next() {
		t := Model{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.ActiveVersion, &t.Archived, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

func (r *PostgresRepository) Update(ctx context.Context, t *Model) error {
	_, err := r.db.Exec(ctx,
		`UPDATE templates SET name=$1, description=$2, active_version=$3, archived=$4, updated_at=NOW() WHERE id=$5`,
		t.Name, t.Description, t.ActiveVersion, t.Archived, t.ID,
	)
	return err
}

func (r *PostgresRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM templates WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *PostgresRepository) CreateVersion(ctx context.Context, v *Version) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO template_versions (template_id, version, subject, html_body, text_body, variables)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		v.TemplateID, v.VersionNum, v.Subject, v.HTMLBody, v.TextBody, v.Variables,
	).Scan(&v.ID, &v.CreatedAt)
}

func (r *PostgresRepository) GetVersions(ctx context.Context, templateID uuid.UUID) ([]Version, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, template_id, version, subject, html_body, text_body, variables, created_at
		 FROM template_versions WHERE template_id = $1 ORDER BY version DESC`, templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		v := Version{}
		if err := rows.Scan(&v.ID, &v.TemplateID, &v.VersionNum, &v.Subject, &v.HTMLBody, &v.TextBody, &v.Variables, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

func (r *PostgresRepository) GetActiveVersion(ctx context.Context, templateID uuid.UUID) (*Version, error) {
	t, err := r.GetByID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	return r.GetVersion(ctx, templateID, t.ActiveVersion)
}

func (r *PostgresRepository) GetVersion(ctx context.Context, templateID uuid.UUID, version int) (*Version, error) {
	v := &Version{}
	err := r.db.QueryRow(ctx,
		`SELECT id, template_id, version, subject, html_body, text_body, variables, created_at
		 FROM template_versions WHERE template_id = $1 AND version = $2`, templateID, version,
	).Scan(&v.ID, &v.TemplateID, &v.VersionNum, &v.Subject, &v.HTMLBody, &v.TextBody, &v.Variables, &v.CreatedAt)
	return v, err
}
