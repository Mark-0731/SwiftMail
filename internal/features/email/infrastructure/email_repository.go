package infrastructure

import (
	"context"

	emailtypes "github.com/Mark-0731/SwiftMail/internal/features/email"
	"github.com/google/uuid"
)

// EmailRepository defines the email log data access interface.
// This interface allows for easy testing and swapping implementations.
type EmailRepository interface {
	Create(ctx context.Context, e *emailtypes.Model) error
	GetByID(ctx context.Context, id uuid.UUID) (*emailtypes.Model, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to string, smtpResponse *string) error
	Search(ctx context.Context, q *emailtypes.LogQuery) ([]emailtypes.Model, int64, error)
	IncrementRetry(ctx context.Context, id uuid.UUID) error
	SetOpened(ctx context.Context, id uuid.UUID) error
	SetClicked(ctx context.Context, id uuid.UUID) error
}
