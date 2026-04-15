package dsn

import (
	"regexp"
	"strings"
)

// BounceType represents the type of email bounce.
type BounceType string

const (
	BounceHard      BounceType = "hard"
	BounceSoft      BounceType = "soft"
	BounceComplaint BounceType = "complaint"
	BounceUnknown   BounceType = "unknown"
)

// BounceResult holds parsed bounce information.
type BounceResult struct {
	Type       BounceType
	Code       string
	Diagnostic string
	Recipient  string
}

var (
	statusCodeRegex = regexp.MustCompile(`(\d)\.\d+\.\d+`)
	smtpCodeRegex   = regexp.MustCompile(`(\d{3})[\s-]`)
)

// Parse analyzes an SMTP response or DSN message and classifies the bounce.
func Parse(smtpResponse string) BounceResult {
	result := BounceResult{
		Diagnostic: smtpResponse,
		Type:       BounceUnknown,
	}

	// Extract SMTP status code (e.g., "550", "421")
	if matches := smtpCodeRegex.FindStringSubmatch(smtpResponse); len(matches) > 1 {
		result.Code = matches[1]
	}

	// Extract enhanced status code (e.g., "5.1.1")
	if matches := statusCodeRegex.FindStringSubmatch(smtpResponse); len(matches) > 1 {
		result.Code = matches[0]
	}

	// Classify based on code
	if strings.HasPrefix(result.Code, "5") {
		result.Type = BounceHard
	} else if strings.HasPrefix(result.Code, "4") {
		result.Type = BounceSoft
	}

	// Check for complaint indicators
	lower := strings.ToLower(smtpResponse)
	if strings.Contains(lower, "spam") ||
		strings.Contains(lower, "complaint") ||
		strings.Contains(lower, "abuse") ||
		strings.Contains(lower, "junk") {
		result.Type = BounceComplaint
	}

	// Check for specific hard bounce reasons
	if strings.Contains(lower, "user unknown") ||
		strings.Contains(lower, "no such user") ||
		strings.Contains(lower, "mailbox not found") ||
		strings.Contains(lower, "account disabled") ||
		strings.Contains(lower, "address rejected") {
		result.Type = BounceHard
	}

	// Check for soft bounce reasons
	if strings.Contains(lower, "try again") ||
		strings.Contains(lower, "temporarily") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many") ||
		strings.Contains(lower, "service unavailable") ||
		strings.Contains(lower, "mailbox full") {
		result.Type = BounceSoft
	}

	return result
}

// IsHardBounce returns true if the response indicates a permanent failure.
func IsHardBounce(smtpCode string) bool {
	return strings.HasPrefix(smtpCode, "5")
}

// IsSoftBounce returns true if the response indicates a temporary failure.
func IsSoftBounce(smtpCode string) bool {
	return strings.HasPrefix(smtpCode, "4")
}
