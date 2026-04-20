package verification

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines verification data operations
type Repository interface {
	// Token operations
	CreateVerificationToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetVerificationToken(ctx context.Context, tokenHash string) (*VerificationToken, error)
	MarkTokenUsed(ctx context.Context, tokenHash string) error
	DeleteToken(ctx context.Context, tokenHash string) error

	// User operations
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error
	GetUserEmail(ctx context.Context, userID uuid.UUID) (email string, name string, verified bool, err error)
}
