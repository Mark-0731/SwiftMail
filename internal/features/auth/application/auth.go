package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Mark-0731/SwiftMail/internal/features/auth"
	"github.com/Mark-0731/SwiftMail/internal/features/auth/domain"
)

// AuthService handles authentication operations
type AuthService interface {
	Signup(ctx context.Context, req *auth.SignupRequest) (*auth.AuthResponse, error)
	Login(ctx context.Context, req *auth.LoginRequest) (*auth.AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*auth.AuthResponse, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*auth.UserResponse, error)
}

// Signup creates a new user account
func (s *service) Signup(ctx context.Context, req *auth.SignupRequest) (*auth.AuthResponse, error) {
	// Check if user exists
	existing, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, ErrUserExists
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check user: %w", err)
	}

	// Hash password
	hash, err := domain.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user, err := s.repo.CreateUser(ctx, req.Email, hash, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	accessToken, refreshToken, err := s.generateTokenPair(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", user.ID.String()).
		Str("email", user.Email).
		Msg("user signed up")

	return &auth.AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

// Login authenticates a user
func (s *service) Login(ctx context.Context, req *auth.LoginRequest) (*auth.AuthResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check account status
	if user.Status == "suspended" || user.Status == "banned" {
		return nil, ErrUserSuspended
	}

	// Verify password
	if !domain.CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check 2FA if enabled
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			return nil, ErrTOTPRequired
		}
		if user.TOTPSecret != nil && !s.totp.ValidateWithSkew(*user.TOTPSecret, req.TOTPCode) {
			return nil, ErrTOTPInvalid
		}
	}

	// Generate tokens
	accessToken, refreshToken, err := s.generateTokenPair(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", user.ID.String()).
		Msg("user logged in")

	return &auth.AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

// RefreshToken generates new tokens from refresh token
func (s *service) RefreshToken(ctx context.Context, refreshToken string) (*auth.AuthResponse, error) {
	claims, err := s.jwt.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Generate new token pair
	accessToken, newRefreshToken, err := s.generateTokenPair(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &auth.AuthResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.jwt.AccessExpiry(),
	}, nil
}

// GetProfile retrieves user profile
func (s *service) GetProfile(ctx context.Context, userID uuid.UUID) (*auth.UserResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	resp := toUserResponse(user)
	return &resp, nil
}

// generateTokenPair generates access and refresh tokens
func (s *service) generateTokenPair(userID uuid.UUID, email, role string) (string, string, error) {
	accessToken, err := s.jwt.GenerateAccessToken(userID, email, role)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(userID, email, role)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}
