package domain

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// KeyRotationManager handles automatic JWT secret rotation.
type KeyRotationManager struct {
	currentSecret  string
	previousSecret string
	mu             sync.RWMutex
	rotationPeriod time.Duration
	logger         zerolog.Logger
	stopChan       chan struct{}
}

// NewKeyRotationManager creates a new key rotation manager.
func NewKeyRotationManager(initialSecret string, rotationPeriod time.Duration, logger zerolog.Logger) *KeyRotationManager {
	return &KeyRotationManager{
		currentSecret:  initialSecret,
		previousSecret: "",
		rotationPeriod: rotationPeriod,
		logger:         logger,
		stopChan:       make(chan struct{}),
	}
}

// Start begins the automatic key rotation process.
func (k *KeyRotationManager) Start() {
	go func() {
		ticker := time.NewTicker(k.rotationPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				k.RotateKey()
			case <-k.stopChan:
				return
			}
		}
	}()
	k.logger.Info().Dur("rotation_period", k.rotationPeriod).Msg("JWT key rotation started")
}

// Stop stops the key rotation process.
func (k *KeyRotationManager) Stop() {
	close(k.stopChan)
	k.logger.Info().Msg("JWT key rotation stopped")
}

// RotateKey generates a new secret and keeps the old one for validation.
func (k *KeyRotationManager) RotateKey() {
	newSecret := generateSecret(64)

	k.mu.Lock()
	k.previousSecret = k.currentSecret
	k.currentSecret = newSecret
	k.mu.Unlock()

	k.logger.Info().Msg("JWT secret rotated successfully")
}

// GetCurrentSecret returns the current secret for signing new tokens.
func (k *KeyRotationManager) GetCurrentSecret() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.currentSecret
}

// GetPreviousSecret returns the previous secret for validating old tokens.
func (k *KeyRotationManager) GetPreviousSecret() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.previousSecret
}

// ValidateWithBothSecrets tries to validate a token with both current and previous secrets.
func (k *KeyRotationManager) ValidateWithBothSecrets(tokenString string, jwtManager *JWTManager) (*Claims, error) {
	// Try current secret first
	claims, err := jwtManager.ValidateAccessToken(tokenString)
	if err == nil {
		return claims, nil
	}

	// Try previous secret if current fails
	if k.GetPreviousSecret() != "" {
		// Create temporary JWT manager with previous secret
		prevJWT := NewJWTManager(
			k.GetPreviousSecret(),
			string(jwtManager.refreshSecret),
			jwtManager.accessExpiry,
			jwtManager.refreshExpiry,
		)
		claims, err = prevJWT.ValidateAccessToken(tokenString)
		if err == nil {
			k.logger.Debug().Msg("token validated with previous secret")
			return claims, nil
		}
	}

	return nil, ErrInvalidToken
}

func generateSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
