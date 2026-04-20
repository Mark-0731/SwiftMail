package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines user data access interface
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	ListAll(ctx context.Context, page, perPage int) ([]User, int64, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, name string) error
	Suspend(ctx context.Context, id uuid.UUID) error
	Unsuspend(ctx context.Context, id uuid.UUID) error
}
