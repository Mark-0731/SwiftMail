package pipeline

import (
	"context"
	"fmt"

	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/cache"
	"github.com/Mark-0731/SwiftMail/pkg/validator"
	"github.com/rs/zerolog"
)

// ValidationStage validates email format, suppression, and credits.
type ValidationStage struct {
	cache         cache.Cache
	creditService *billing.CreditService
	logger        zerolog.Logger
}

// NewValidationStage creates a new validation stage.
func NewValidationStage(cache cache.Cache, creditService *billing.CreditService, logger zerolog.Logger) Stage {
	return &ValidationStage{
		cache:         cache,
		creditService: creditService,
		logger:        logger,
	}
}

// Name returns the stage name.
func (s *ValidationStage) Name() string {
	return "validation"
}

// Execute performs validation checks.
func (s *ValidationStage) Execute(ctx context.Context, state *State) error {
	// 1. Email format validation
	toValidation := validator.ValidateEmailAdvanced(state.To, false)
	if !toValidation.Valid {
		return fmt.Errorf("invalid recipient email: %s", toValidation.Reason)
	}

	fromValidation := validator.ValidateEmailAdvanced(state.From, false)
	if !fromValidation.Valid {
		return fmt.Errorf("invalid sender email: %s", fromValidation.Reason)
	}

	// Log warnings for disposable/role-based emails
	if toValidation.IsDisposable {
		s.logger.Warn().Str("email", state.To).Msg("sending to disposable email address")
	}
	if toValidation.IsRoleBased {
		s.logger.Info().Str("email", state.To).Msg("sending to role-based email address")
	}

	// 2. Suppression check
	suppressed, err := s.cache.IsSuppressed(ctx, state.UserID, state.To)
	if err != nil {
		s.logger.Warn().Err(err).Msg("suppression check failed")
	} else if suppressed {
		return fmt.Errorf("recipient %s is suppressed", state.To)
	}

	// 3. Credit check
	hasCredits, currentBalance, err := s.creditService.CheckCreditAvailability(ctx, state.UserID, 1)
	if err != nil {
		s.logger.Warn().Err(err).Str("user_id", state.UserID.String()).Msg("credit check failed, allowing request")
	} else if !hasCredits {
		return fmt.Errorf("insufficient credits (current balance: %d)", currentBalance)
	}

	return nil
}
