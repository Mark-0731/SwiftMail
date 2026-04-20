package pipeline

import (
	"context"
	"fmt"

	"github.com/Mark-0731/SwiftMail/internal/billing"
	"github.com/Mark-0731/SwiftMail/internal/infrastructure/cache"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// PersistenceStage handles database persistence and credit reservation.
type PersistenceStage struct {
	repo          EmailRepository
	cache         cache.Cache
	creditService *billing.CreditService
	logger        zerolog.Logger
}

// NewPersistenceStage creates a new persistence stage.
func NewPersistenceStage(
	repo EmailRepository,
	cache cache.Cache,
	creditService *billing.CreditService,
	logger zerolog.Logger,
) Stage {
	return &PersistenceStage{
		repo:          repo,
		cache:         cache,
		creditService: creditService,
		logger:        logger,
	}
}

// Name returns the stage name.
func (s *PersistenceStage) Name() string {
	return "persistence"
}

// Execute persists the email log and reserves credits.
func (s *PersistenceStage) Execute(ctx context.Context, state *State) error {
	// 1. Generate message ID
	state.MessageID = fmt.Sprintf("<%s@swiftmail>", uuid.New().String())

	// 2. Look up domain ID
	fromDomain := extractDomain(state.From)
	if fromDomain != "" {
		domainID, err := s.cache.GetDomainID(ctx, state.UserID, fromDomain)
		if err != nil {
			s.logger.Warn().Err(err).Str("domain", fromDomain).Msg("failed to get domain ID from cache")
		} else {
			state.DomainID = domainID
		}
	}

	// 3. Create email log
	emailLog := &EmailModel{
		UserID:     state.UserID,
		DomainID:   state.DomainID,
		MessageID:  state.MessageID,
		FromEmail:  state.From,
		ToEmail:    state.To,
		Subject:    state.RenderedSubject,
		Status:     "queued",
		TemplateID: state.TemplateID,
		Tags:       state.Tags,
		MaxRetries: 3,
	}

	if state.IdempotencyKey != "" {
		emailLog.IdempotencyKey = &state.IdempotencyKey
	}

	if err := s.repo.Create(ctx, emailLog); err != nil {
		return fmt.Errorf("failed to create email log: %w", err)
	}

	state.EmailLogID = emailLog.ID

	// 4. Reserve credit
	if err := s.creditService.ReserveCreditForSend(ctx, state.UserID, emailLog.ID, 1); err != nil {
		s.logger.Warn().Err(err).Str("email_id", emailLog.ID.String()).Msg("failed to reserve credit, continuing anyway")
	} else {
		state.CreditReserved = true
	}

	s.logger.Info().
		Str("email_log_id", emailLog.ID.String()).
		Str("message_id", state.MessageID).
		Msg("email log created and credit reserved")

	return nil
}
