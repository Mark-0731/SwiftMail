package provider

import (
	"context"

	"github.com/rs/zerolog"
)

// Selector chooses which provider to use for sending.
type Selector struct {
	primary  Provider
	fallback Provider
	logger   zerolog.Logger
}

// NewSelector creates a new provider selector.
func NewSelector(primary, fallback Provider, logger zerolog.Logger) *Selector {
	return &Selector{
		primary:  primary,
		fallback: fallback,
		logger:   logger,
	}
}

// Send sends an email using the primary provider, with fallback on failure.
func (s *Selector) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	// Try primary provider
	resp, err := s.primary.Send(ctx, req)
	if err == nil {
		return resp, nil
	}

	// Log primary failure
	s.logger.Warn().
		Err(err).
		Str("primary_provider", s.primary.Name()).
		Str("to", req.To).
		Msg("primary provider failed, trying fallback")

	// Try fallback if available
	if s.fallback != nil {
		resp, err = s.fallback.Send(ctx, req)
		if err == nil {
			s.logger.Info().
				Str("fallback_provider", s.fallback.Name()).
				Str("to", req.To).
				Msg("email sent via fallback provider")
			return resp, nil
		}

		s.logger.Error().
			Err(err).
			Str("fallback_provider", s.fallback.Name()).
			Str("to", req.To).
			Msg("fallback provider also failed")
	}

	// Both failed
	return resp, err
}

// Name returns the selector name.
func (s *Selector) Name() string {
	return "selector"
}
