package pipeline

import (
	"context"
	"fmt"

	"github.com/Mark-0731/SwiftMail/internal/template"
	"github.com/rs/zerolog"
)

// RenderStage handles template rendering.
type RenderStage struct {
	templateSvc template.Service
	logger      zerolog.Logger
}

// NewRenderStage creates a new render stage.
func NewRenderStage(templateSvc template.Service, logger zerolog.Logger) Stage {
	return &RenderStage{
		templateSvc: templateSvc,
		logger:      logger,
	}
}

// Name returns the stage name.
func (s *RenderStage) Name() string {
	return "render"
}

// Execute performs template rendering.
func (s *RenderStage) Execute(ctx context.Context, state *State) error {
	// If no template ID, use original content
	if state.TemplateID == nil {
		state.RenderedSubject = state.Subject
		state.RenderedHTML = state.HTML
		state.RenderedText = state.Text
		return nil
	}

	// Render template
	subject, html, text, err := s.templateSvc.Preview(ctx, *state.TemplateID, state.Variables)
	if err != nil {
		return fmt.Errorf("template rendering failed: %w", err)
	}

	state.RenderedSubject = subject
	state.RenderedHTML = html
	state.RenderedText = text

	s.logger.Debug().
		Str("user_id", state.UserID.String()).
		Str("template_id", state.TemplateID.String()).
		Msg("template rendered successfully")

	return nil
}
