package application

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/infrastructure"
)

// Common errors
var (
	ErrUserExists         = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserSuspended      = errors.New("account is suspended")
	ErrTOTPRequired       = errors.New("2FA code required")
	ErrTOTPInvalid        = errors.New("invalid 2FA code")
	ErrAPIKeyNotFound     = errors.New("API key not found")
	ErrAccountLocked      = errors.New("account locked due to too many failed attempts")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded, please try again later")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// Service defines the auth business logic interface
type Service interface {
	AuthService
	TOTPService
	APIKeyService
	PasswordService
	SessionService

	// Verification methods (delegated to verification service)
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, token string) error
}

// service implements all auth-related use cases
type service struct {
	repo            infrastructure.Repository
	jwt             *domain.JWTManager
	totp            *domain.TOTPManager
	apiKey          *domain.APIKeyManager
	rdb             *redis.Client
	logger          zerolog.Logger
	verificationSvc VerificationService // Injected verification service
}

// VerificationService defines email verification operations (external dependency)
type VerificationService interface {
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, token string) error
}

// NewService creates a new auth service
func NewService(
	repo infrastructure.Repository,
	jwt *domain.JWTManager,
	totp *domain.TOTPManager,
	apiKey *domain.APIKeyManager,
	rdb *redis.Client,
	logger zerolog.Logger,
	verificationSvc VerificationService,
) Service {
	return &service{
		repo:            repo,
		jwt:             jwt,
		totp:            totp,
		apiKey:          apiKey,
		rdb:             rdb,
		logger:          logger,
		verificationSvc: verificationSvc,
	}
}
