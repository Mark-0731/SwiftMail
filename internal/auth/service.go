package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

var (
	ErrUserExists         = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserSuspended      = errors.New("account is suspended")
	ErrTOTPRequired       = errors.New("2FA code required")
	ErrTOTPInvalid        = errors.New("invalid 2FA code")
	ErrAPIKeyNotFound     = errors.New("API key not found")
)

// Service defines the auth business logic interface.
type Service interface {
	Signup(ctx context.Context, req *SignupRequest) (*AuthResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error)
	SetupTOTP(ctx context.Context, userID uuid.UUID) (*TOTPSetupResponse, error)
	VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error
	CreateAPIKey(ctx context.Context, userID uuid.UUID, req *CreateAPIKeyRequest) (*APIKeyResponse, error)
	ListAPIKeys(ctx context.Context, userID uuid.UUID) (*APIKeyListResponse, error)
	DeleteAPIKey(ctx context.Context, keyID, userID uuid.UUID) error
}

type service struct {
	repo   Repository
	jwt    *JWTManager
	totp   *TOTPManager
	apiKey *APIKeyManager
	logger zerolog.Logger
}

// NewService creates a new auth service.
func NewService(repo Repository, jwt *JWTManager, totp *TOTPManager, apiKey *APIKeyManager, logger zerolog.Logger) Service {
	return &service{
		repo:   repo,
		jwt:    jwt,
		totp:   totp,
		apiKey: apiKey,
		logger: logger,
	}
}

func (s *service) Signup(ctx context.Context, req *SignupRequest) (*AuthResponse, error) {
	// Check if user exists
	existing, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, ErrUserExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check user: %w", err)
	}

	// Hash password
	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user, err := s.repo.CreateUser(ctx, req.Email, hash, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	s.logger.Info().Str("user_id", user.ID.String()).Str("email", user.Email).Msg("user signed up")

	return &AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

func (s *service) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check status
	if user.Status == "suspended" || user.Status == "banned" {
		return nil, ErrUserSuspended
	}

	// Check password
	if !CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check 2FA
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			return nil, ErrTOTPRequired
		}
		if user.TOTPSecret != nil && !s.totp.ValidateWithSkew(*user.TOTPSecret, req.TOTPCode) {
			return nil, ErrTOTPInvalid
		}
	}

	// Generate tokens
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	s.logger.Info().Str("user_id", user.ID.String()).Msg("user logged in")

	return &AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

func (s *service) RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	claims, err := s.jwt.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshToken, err := s.jwt.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

func (s *service) GetProfile(ctx context.Context, userID uuid.UUID) (*UserResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	resp := toUserResponse(user)
	return &resp, nil
}

func (s *service) SetupTOTP(ctx context.Context, userID uuid.UUID) (*TOTPSetupResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	secret, url, err := s.totp.GenerateSecret(user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP: %w", err)
	}

	if err := s.repo.UpdateTOTPSecret(ctx, userID, secret); err != nil {
		return nil, fmt.Errorf("failed to store TOTP secret: %w", err)
	}

	return &TOTPSetupResponse{
		Secret: secret,
		URL:    url,
	}, nil
}

func (s *service) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if user.TOTPSecret == nil {
		return fmt.Errorf("TOTP not set up")
	}

	if !s.totp.ValidateWithSkew(*user.TOTPSecret, code) {
		return ErrTOTPInvalid
	}

	return s.repo.EnableTOTP(ctx, userID)
}

func (s *service) CreateAPIKey(ctx context.Context, userID uuid.UUID, req *CreateAPIKeyRequest) (*APIKeyResponse, error) {
	rawKey, keyHash, prefix, err := s.apiKey.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	key, err := s.repo.CreateAPIKey(ctx, userID, req.Name, keyHash, prefix, req.Permissions, req.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Get user to cache role and status
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Cache the API key in Redis for fast authentication
	cacheData := &CachedAPIKeyData{
		UserID:      userID,
		Role:        user.Role,
		Status:      user.Status,
		RatePerSec:  100, // Default rate limits
		RatePerDay:  50000,
		Permissions: req.Permissions,
	}
	if err := s.apiKey.CacheAPIKey(ctx, keyHash, cacheData); err != nil {
		s.logger.Warn().Err(err).Msg("failed to cache API key (will work after first use)")
	}

	s.logger.Info().Str("user_id", userID.String()).Str("key_prefix", prefix).Msg("API key created")

	return &APIKeyResponse{
		ID:          key.ID,
		Name:        key.Name,
		KeyPrefix:   key.KeyPrefix,
		Key:         rawKey, // Only returned once
		Permissions: req.Permissions,
		LastUsedAt:  key.LastUsedAt,
		ExpiresAt:   key.ExpiresAt,
		CreatedAt:   key.CreatedAt,
	}, nil
}

func (s *service) ListAPIKeys(ctx context.Context, userID uuid.UUID) (*APIKeyListResponse, error) {
	keys, err := s.repo.GetAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	var resp []APIKeyResponse
	for _, k := range keys {
		resp = append(resp, APIKeyResponse{
			ID:          k.ID,
			Name:        k.Name,
			KeyPrefix:   k.KeyPrefix,
			Permissions: k.Permissions,
			LastUsedAt:  k.LastUsedAt,
			ExpiresAt:   k.ExpiresAt,
			CreatedAt:   k.CreatedAt,
		})
	}

	return &APIKeyListResponse{Keys: resp}, nil
}

func (s *service) DeleteAPIKey(ctx context.Context, keyID, userID uuid.UUID) error {
	if err := s.repo.DeleteAPIKey(ctx, keyID, userID); err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	s.logger.Info().Str("user_id", userID.String()).Str("key_id", keyID.String()).Msg("API key deleted")
	return nil
}

func toUserResponse(u *UserModel) UserResponse {
	return UserResponse{
		ID:            u.ID,
		Email:         u.Email,
		Name:          u.Name,
		Role:          u.Role,
		TOTPEnabled:   u.TOTPEnabled,
		EmailVerified: u.EmailVerified,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt,
	}
}
