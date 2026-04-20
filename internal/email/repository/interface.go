package repository

import (
	"context"

	"github.com/Mark-0731/SwiftMail/internal/email"
	"github.com/google/uuid"
)

// Repository defines the email log data access interface.
// This interface allows for easy testing and swapping implementations.
type Repository interface {
	Create(ctx context.Context, e *email.Model) error
	GetByID(ctx context.Context, id uuid.UUID) (*email.Model, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to string, smtpResponse *string) error
	Search(ctx context.Context, q *email.LogQuery) ([]email.Model, int64, error)
	IncrementRetry(ctx context.Context, id uuid.UUID) error
	SetOpened(ctx context.Context, id uuid.UUID) error
	SetClicked(ctx context.Context, id uuid.UUID) error
}
