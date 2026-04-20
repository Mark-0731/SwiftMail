package template

import (
	"time"

	"github.com/google/uuid"
)

// Model represents a template.
type Model struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	ActiveVersion int       `json:"active_version"`
	Archived      bool      `json:"archived"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Version represents a template version.
type Version struct {
	ID         uuid.UUID `json:"id"`
	TemplateID uuid.UUID `json:"template_id"`
	VersionNum int       `json:"version"`
	Subject    string    `json:"subject"`
	HTMLBody   *string   `json:"html_body"`
	TextBody   *string   `json:"text_body"`
	Variables  []string  `json:"variables"`
	CreatedAt  time.Time `json:"created_at"`
}
