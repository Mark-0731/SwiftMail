package template

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// CreateTemplateRequest is the request to create a template.
type CreateTemplateRequest struct {
	Name        string  `json:"name" validate:"required"`
	Description *string `json:"description,omitempty"`
	Subject     string  `json:"subject" validate:"required"`
	HTMLBody    *string `json:"html_body,omitempty"`
	TextBody    *string `json:"text_body,omitempty"`
}

// UpdateTemplateRequest is the request to update a template.
type UpdateTemplateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Subject     *string `json:"subject,omitempty"`
	HTMLBody    *string `json:"html_body,omitempty"`
	TextBody    *string `json:"text_body,omitempty"`
}

// PreviewRequest is the request to preview a rendered template.
type PreviewRequest struct {
	Variables map[string]string `json:"variables"`
}

// Service defines the template business logic interface.
type Service interface {
	Create(ctx context.Context, userID uuid.UUID, req *CreateTemplateRequest) (*Model, error)
	List(ctx context.Context, userID uuid.UUID) ([]Model, error)
	Get(ctx context.Context, id uuid.UUID) (*Model, error)
	Update(ctx context.Context, id uuid.UUID, req *UpdateTemplateRequest) (*Model, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetVersions(ctx context.Context, templateID uuid.UUID) ([]Version, error)
	Preview(ctx context.Context, templateID uuid.UUID, vars map[string]string) (subject, html, text string, err error)
	Duplicate(ctx context.Context, id, userID uuid.UUID) (*Model, error)
	Archive(ctx context.Context, id uuid.UUID) error
	Restore(ctx context.Context, id uuid.UUID) error
	Rollback(ctx context.Context, templateID uuid.UUID, version int) error
	GetActiveVersion(ctx context.Context, templateID uuid.UUID) (*Version, error)
}

type service struct {
	repo   Repository
	logger zerolog.Logger
}

// NewService creates a new template service.
func NewService(repo Repository, logger zerolog.Logger) Service {
	return &service{repo: repo, logger: logger}
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, req *CreateTemplateRequest) (*Model, error) {
	t := &Model{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Create version 1
	vars := ExtractVariables(req.Subject)
	if req.HTMLBody != nil {
		vars = append(vars, ExtractVariables(*req.HTMLBody)...)
	}
	if req.TextBody != nil {
		vars = append(vars, ExtractVariables(*req.TextBody)...)
	}

	v := &Version{
		TemplateID: t.ID,
		VersionNum: 1,
		Subject:    req.Subject,
		HTMLBody:   req.HTMLBody,
		TextBody:   req.TextBody,
		Variables:  vars,
	}

	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("failed to create template version: %w", err)
	}

	return t, nil
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]Model, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *service) Get(ctx context.Context, id uuid.UUID) (*Model, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) Update(ctx context.Context, id uuid.UUID, req *UpdateTemplateRequest) (*Model, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Description != nil {
		t.Description = req.Description
	}

	// If subject or body is being changed, create a new version
	if req.Subject != nil || req.HTMLBody != nil || req.TextBody != nil {
		currentVersion, err := s.repo.GetActiveVersion(ctx, id)
		if err != nil {
			return nil, err
		}

		newVersion := &Version{
			TemplateID: id,
			VersionNum: currentVersion.VersionNum + 1,
			Subject:    currentVersion.Subject,
			HTMLBody:   currentVersion.HTMLBody,
			TextBody:   currentVersion.TextBody,
		}

		if req.Subject != nil {
			newVersion.Subject = *req.Subject
		}
		if req.HTMLBody != nil {
			newVersion.HTMLBody = req.HTMLBody
		}
		if req.TextBody != nil {
			newVersion.TextBody = req.TextBody
		}

		newVersion.Variables = ExtractVariables(newVersion.Subject)
		if newVersion.HTMLBody != nil {
			newVersion.Variables = append(newVersion.Variables, ExtractVariables(*newVersion.HTMLBody)...)
		}

		if err := s.repo.CreateVersion(ctx, newVersion); err != nil {
			return nil, err
		}

		t.ActiveVersion = newVersion.VersionNum
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

func (s *service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}

func (s *service) GetVersions(ctx context.Context, templateID uuid.UUID) ([]Version, error) {
	return s.repo.GetVersions(ctx, templateID)
}

func (s *service) Preview(ctx context.Context, templateID uuid.UUID, vars map[string]string) (string, string, string, error) {
	v, err := s.repo.GetActiveVersion(ctx, templateID)
	if err != nil {
		return "", "", "", err
	}

	subject := Render(v.Subject, vars)
	html := ""
	if v.HTMLBody != nil {
		html = Render(*v.HTMLBody, vars)
	}
	text := ""
	if v.TextBody != nil {
		text = Render(*v.TextBody, vars)
	}

	return subject, html, text, nil
}

func (s *service) Duplicate(ctx context.Context, id, userID uuid.UUID) (*Model, error) {
	original, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	t := &Model{
		UserID:      userID,
		Name:        original.Name + " (copy)",
		Description: original.Description,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}

	versions, err := s.repo.GetVersions(ctx, id)
	if err != nil {
		return nil, err
	}

	for _, v := range versions {
		newV := &Version{
			TemplateID: t.ID,
			VersionNum: v.VersionNum,
			Subject:    v.Subject,
			HTMLBody:   v.HTMLBody,
			TextBody:   v.TextBody,
			Variables:  v.Variables,
		}
		if err := s.repo.CreateVersion(ctx, newV); err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (s *service) Archive(ctx context.Context, id uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	t.Archived = true
	return s.repo.Update(ctx, t)
}

func (s *service) Restore(ctx context.Context, id uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	t.Archived = false
	return s.repo.Update(ctx, t)
}

func (s *service) Rollback(ctx context.Context, templateID uuid.UUID, version int) error {
	_, err := s.repo.GetVersion(ctx, templateID, version)
	if err != nil {
		return fmt.Errorf("version %d not found: %w", version, err)
	}

	t, err := s.repo.GetByID(ctx, templateID)
	if err != nil {
		return err
	}

	t.ActiveVersion = version
	return s.repo.Update(ctx, t)
}

func (s *service) GetActiveVersion(ctx context.Context, templateID uuid.UUID) (*Version, error) {
	return s.repo.GetActiveVersion(ctx, templateID)
}
