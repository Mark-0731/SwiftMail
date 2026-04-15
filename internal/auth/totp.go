package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPManager handles TOTP 2FA operations.
type TOTPManager struct{}

// NewTOTPManager creates a new TOTP manager.
func NewTOTPManager() *TOTPManager {
	return &TOTPManager{}
}

// GenerateSecret generates a new TOTP secret for a user.
func (t *TOTPManager) GenerateSecret(email string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SwiftMail",
		AccountName: email,
		Period:      30,
		SecretSize:  32,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// ValidateCode validates a TOTP code against the stored secret.
func (t *TOTPManager) ValidateCode(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes generates a set of one-time backup codes.
func (t *TOTPManager) GenerateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		codes[i] = base32.StdEncoding.EncodeToString(b)[:8]
	}
	return codes, nil
}

// ValidateWithSkew validates a TOTP code with a time window skew.
func (t *TOTPManager) ValidateWithSkew(secret, code string) bool {
	valid, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:     1, // Allow 1 period skew (30 seconds)
		Digits:   otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}
