package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
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
	ErrAccountLocked      = errors.New("account locked due to too many failed attempts")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded, please try again later")
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

	// Password reset
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error

	// Email verification
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, token string) error

	// Session management
	RevokeAllSessions(ctx context.Context, userID uuid.UUID) error
	RevokeSession(ctx context.Context, refreshToken string) error

	// Password change
	ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error
}

type service struct {
	repo   Repository
	jwt    *JWTManager
	totp   *TOTPManager
	apiKey *APIKeyManager
	logger zerolog.Logger
	rdb    redis.Client // For rate limiting and session management
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

func (s *service) RequestPasswordReset(ctx context.Context, email string) error {
	// Check rate limit (max 3 requests per hour per email)
	rateLimitKey := fmt.Sprintf("password_reset_rate:%s", email)
	count, err := s.rdb.Incr(ctx, rateLimitKey).Result()
	if err == nil {
		if count == 1 {
			s.rdb.Expire(ctx, rateLimitKey, 1*time.Hour)
		}
		if count > 3 {
			return ErrRateLimitExceeded
		}
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		// Don't reveal if email exists
		s.logger.Info().Str("email", email).Msg("password reset requested for non-existent email")
		return nil
	}

	// Generate reset token (32 bytes = 64 hex chars)
	token := generateSecureToken(32)
	tokenHash := hashToken(token)

	// Store token in database with 1 hour expiry
	if err := s.repo.CreatePasswordResetToken(ctx, user.ID, tokenHash, time.Now().Add(1*time.Hour)); err != nil {
		return fmt.Errorf("failed to create reset token: %w", err)
	}

	// TODO: Send email with reset link
	// resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	// sendEmail(user.Email, "Password Reset", resetLink)

	s.logger.Info().Str("user_id", user.ID.String()).Msg("password reset token generated")
	return nil
}

func (s *service) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := hashToken(token)

	// Get token from database
	resetToken, err := s.repo.GetPasswordResetToken(ctx, tokenHash)
	if err != nil {
		return ErrInvalidToken
	}

	// Check if token is expired
	if time.Now().After(resetToken.ExpiresAt) {
		s.repo.DeletePasswordResetToken(ctx, tokenHash)
		return ErrInvalidToken
	}

	// Check if token is already used
	if resetToken.UsedAt != nil {
		return ErrInvalidToken
	}

	// Hash new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, resetToken.UserID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	if err := s.repo.MarkPasswordResetTokenUsed(ctx, tokenHash); err != nil {
		s.logger.Warn().Err(err).Msg("failed to mark token as used")
	}

	// Revoke all sessions for security
	s.RevokeAllSessions(ctx, resetToken.UserID)

	s.logger.Info().Str("user_id", resetToken.UserID.String()).Msg("password reset successful")
	return nil
}

func (s *service) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	if user.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Check rate limit (max 3 requests per hour)
	rateLimitKey := fmt.Sprintf("email_verify_rate:%s", userID.String())
	count, err := s.rdb.Incr(ctx, rateLimitKey).Result()
	if err == nil {
		if count == 1 {
			s.rdb.Expire(ctx, rateLimitKey, 1*time.Hour)
		}
		if count > 3 {
			return ErrRateLimitExceeded
		}
	}

	// Generate verification token
	token := generateSecureToken(32)
	tokenHash := hashToken(token)

	// Store token with 24 hour expiry
	if err := s.repo.CreateEmailVerificationToken(ctx, userID, tokenHash, time.Now().Add(24*time.Hour)); err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	// TODO: Send verification email
	// verifyLink := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
	// sendEmail(user.Email, "Verify Your Email", verifyLink)

	s.logger.Info().Str("user_id", userID.String()).Msg("verification email sent")
	return nil
}

func (s *service) VerifyEmail(ctx context.Context, token string) error {
	tokenHash := hashToken(token)

	// Get token from database
	verifyToken, err := s.repo.GetEmailVerificationToken(ctx, tokenHash)
	if err != nil {
		return ErrInvalidToken
	}

	// Check if token is expired
	if time.Now().After(verifyToken.ExpiresAt) {
		s.repo.DeleteEmailVerificationToken(ctx, tokenHash)
		return ErrInvalidToken
	}

	// Check if already used
	if verifyToken.UsedAt != nil {
		return ErrInvalidToken
	}

	// Mark email as verified
	if err := s.repo.MarkEmailVerified(ctx, verifyToken.UserID); err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	// Mark token as used
	if err := s.repo.MarkEmailVerificationTokenUsed(ctx, tokenHash); err != nil {
		s.logger.Warn().Err(err).Msg("failed to mark token as used")
	}

	s.logger.Info().Str("user_id", verifyToken.UserID.String()).Msg("email verified")
	return nil
}

func (s *service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	// Increment user's token version to invalidate all existing tokens
	if err := s.repo.IncrementTokenVersion(ctx, userID); err != nil {
		return fmt.Errorf("failed to revoke sessions: %w", err)
	}

	// Clear all refresh tokens from Redis
	pattern := fmt.Sprintf("refresh_token:%s:*", userID.String())
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err == nil && len(keys) > 0 {
		s.rdb.Del(ctx, keys...)
	}

	s.logger.Info().Str("user_id", userID.String()).Msg("all sessions revoked")
	return nil
}

func (s *service) RevokeSession(ctx context.Context, refreshToken string) error {
	claims, err := s.jwt.ValidateRefreshToken(refreshToken)
	if err != nil {
		return ErrInvalidToken
	}

	// Add token to blacklist
	tokenKey := fmt.Sprintf("blacklist:refresh:%s", refreshToken)
	s.rdb.Set(ctx, tokenKey, "1", s.jwt.RefreshExpiry())

	s.logger.Info().Str("user_id", claims.UserID.String()).Msg("session revoked")
	return nil
}

func (s *service) LoginWithProtection(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Check if account is locked
	lockKey := fmt.Sprintf("account_lock:%s", req.Email)
	locked, err := s.rdb.Get(ctx, lockKey).Result()
	if err == nil && locked == "1" {
		return nil, ErrAccountLocked
	}

	// Check login rate limit (max 5 attempts per 15 minutes)
	rateLimitKey := fmt.Sprintf("login_rate:%s", req.Email)
	attempts, err := s.rdb.Incr(ctx, rateLimitKey).Result()
	if err == nil {
		if attempts == 1 {
			s.rdb.Expire(ctx, rateLimitKey, 15*time.Minute)
		}
		if attempts > 5 {
			// Lock account for 30 minutes
			s.rdb.Set(ctx, lockKey, "1", 30*time.Minute)
			s.logger.Warn().Str("email", req.Email).Msg("account locked due to too many failed attempts")
			return nil, ErrAccountLocked
		}
	}

	// Attempt login
	response, err := s.Login(ctx, req)
	if err != nil {
		// Increment failed attempts counter
		failKey := fmt.Sprintf("login_fail:%s", req.Email)
		failCount, _ := s.rdb.Incr(ctx, failKey).Result()
		if failCount == 1 {
			s.rdb.Expire(ctx, failKey, 1*time.Hour)
		}

		// Log failed attempt
		s.logger.Warn().Str("email", req.Email).Int64("fail_count", failCount).Msg("failed login attempt")
		return nil, err
	}

	// Clear rate limit counters on successful login
	s.rdb.Del(ctx, rateLimitKey, fmt.Sprintf("login_fail:%s", req.Email))

	return response, nil
}

func generateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify current password
	if !CheckPassword(currentPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	// Hash new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, userID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Revoke all sessions for security
	s.RevokeAllSessions(ctx, userID)

	s.logger.Info().Str("user_id", userID.String()).Msg("password changed successfully")
	return nil
}
