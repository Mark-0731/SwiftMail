package worker

import (
	"regexp"
	"strings"
	"time"
)

// ErrorClassification represents the classification of an email sending error
type ErrorClassification struct {
	IsTemporary     bool          // Should we retry?
	Category        string        // Error category for metrics/logging
	RetryAfter      time.Duration // Suggested retry delay
	ShouldMoveToDLQ bool          // Should move to DLQ after max retries?
	ErrorCode       string        // SMTP error code (e.g., "550", "5.1.1")
	Description     string        // Human-readable description
}

// Error categories
const (
	CategoryInvalidRecipient = "invalid_recipient" // 5.1.1 - User unknown
	CategoryMailboxFull      = "mailbox_full"      // 552 - Mailbox full
	CategoryRateLimit        = "rate_limit"        // 421 4.7.0 - Rate limited
	CategoryBlocked          = "blocked"           // 554 - IP/domain blocked
	CategorySpam             = "spam"              // 554 - Spam detected
	CategoryTemporary        = "temporary"         // 4xx - Generic temporary
	CategoryPermanent        = "permanent"         // 5xx - Generic permanent
	CategoryNetworkError     = "network_error"     // Connection/timeout
	CategoryAuthError        = "auth_error"        // Authentication failed
	CategoryUnknown          = "unknown"           // Unknown error
)

var (
	// Enhanced status code patterns (RFC 3463)
	enhancedStatusRegex = regexp.MustCompile(`([2-5])\.(\d+)\.(\d+)`)

	// SMTP code patterns
	smtpCodeRegex = regexp.MustCompile(`^([2-5]\d{2})`)
)

// ClassifyError analyzes an SMTP error and returns detailed classification
func ClassifyError(errorMsg string) ErrorClassification {
	if errorMsg == "" {
		// Empty error = network/timeout issue
		return ErrorClassification{
			IsTemporary:     true,
			Category:        CategoryNetworkError,
			RetryAfter:      30 * time.Second,
			ShouldMoveToDLQ: false,
			ErrorCode:       "network_error",
			Description:     "Network timeout or connection error",
		}
	}

	errorLower := strings.ToLower(errorMsg)

	// 1. Check for enhanced status codes (most specific)
	if matches := enhancedStatusRegex.FindStringSubmatch(errorMsg); len(matches) == 4 {
		class := matches[1]
		_ = matches[2] // subject (unused but part of enhanced status code)
		_ = matches[3] // detail (unused but part of enhanced status code)
		enhancedCode := matches[0]

		// Parse enhanced status code
		switch enhancedCode {
		// 5.1.1 - User unknown / Invalid recipient
		case "5.1.1":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryInvalidRecipient,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       enhancedCode,
				Description:     "Invalid recipient address",
			}

		// 5.1.2 - Host unknown
		case "5.1.2":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryInvalidRecipient,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       enhancedCode,
				Description:     "Invalid domain",
			}

		// 5.7.1 - Delivery not authorized / Blocked
		case "5.7.1":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryBlocked,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       enhancedCode,
				Description:     "Sender blocked or not authorized",
			}

		// 4.2.1 - Mailbox full (temporary)
		case "4.2.1":
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryMailboxFull,
				RetryAfter:      2 * time.Hour,
				ShouldMoveToDLQ: false,
				ErrorCode:       enhancedCode,
				Description:     "Mailbox full (temporary)",
			}

		// 4.2.2 - Mailbox full (permanent)
		case "5.2.2":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryMailboxFull,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       enhancedCode,
				Description:     "Mailbox full (permanent)",
			}

		// 4.7.0 - Rate limit / Temporary failure
		case "4.7.0":
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryRateLimit,
				RetryAfter:      15 * time.Minute,
				ShouldMoveToDLQ: false,
				ErrorCode:       enhancedCode,
				Description:     "Rate limit exceeded",
			}

		// 5.7.0 - Security policy violation
		case "5.7.0":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryBlocked,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       enhancedCode,
				Description:     "Security policy violation",
			}

		default:
			// Generic classification based on class
			if class == "4" {
				return ErrorClassification{
					IsTemporary:     true,
					Category:        CategoryTemporary,
					RetryAfter:      5 * time.Minute,
					ShouldMoveToDLQ: false,
					ErrorCode:       enhancedCode,
					Description:     "Temporary failure",
				}
			} else if class == "5" {
				return ErrorClassification{
					IsTemporary:     false,
					Category:        CategoryPermanent,
					RetryAfter:      0,
					ShouldMoveToDLQ: true,
					ErrorCode:       enhancedCode,
					Description:     "Permanent failure",
				}
			}
		}
	}

	// 2. Check for SMTP codes (less specific)
	if matches := smtpCodeRegex.FindStringSubmatch(errorMsg); len(matches) == 2 {
		code := matches[1]

		switch code {
		// 421 - Service not available (rate limit or temporary)
		case "421":
			if strings.Contains(errorLower, "rate") || strings.Contains(errorLower, "limit") {
				return ErrorClassification{
					IsTemporary:     true,
					Category:        CategoryRateLimit,
					RetryAfter:      15 * time.Minute,
					ShouldMoveToDLQ: false,
					ErrorCode:       code,
					Description:     "Rate limit exceeded",
				}
			}
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryTemporary,
				RetryAfter:      5 * time.Minute,
				ShouldMoveToDLQ: false,
				ErrorCode:       code,
				Description:     "Service temporarily unavailable",
			}

		// 450 - Mailbox unavailable (temporary)
		case "450":
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryTemporary,
				RetryAfter:      10 * time.Minute,
				ShouldMoveToDLQ: false,
				ErrorCode:       code,
				Description:     "Mailbox temporarily unavailable",
			}

		// 451 - Local error (temporary)
		case "451":
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryTemporary,
				RetryAfter:      5 * time.Minute,
				ShouldMoveToDLQ: false,
				ErrorCode:       code,
				Description:     "Local error in processing",
			}

		// 452 - Insufficient storage (temporary)
		case "452":
			return ErrorClassification{
				IsTemporary:     true,
				Category:        CategoryMailboxFull,
				RetryAfter:      1 * time.Hour,
				ShouldMoveToDLQ: false,
				ErrorCode:       code,
				Description:     "Insufficient storage",
			}

		// 550 - User unknown / Mailbox unavailable (permanent)
		case "550":
			if strings.Contains(errorLower, "user") || strings.Contains(errorLower, "recipient") {
				return ErrorClassification{
					IsTemporary:     false,
					Category:        CategoryInvalidRecipient,
					RetryAfter:      0,
					ShouldMoveToDLQ: true,
					ErrorCode:       code,
					Description:     "Invalid recipient",
				}
			}
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryPermanent,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       code,
				Description:     "Mailbox unavailable",
			}

		// 551 - User not local (permanent)
		case "551":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryInvalidRecipient,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       code,
				Description:     "User not local",
			}

		// 552 - Mailbox full (could be temporary or permanent)
		case "552":
			return ErrorClassification{
				IsTemporary:     true, // Treat as temporary initially
				Category:        CategoryMailboxFull,
				RetryAfter:      2 * time.Hour,
				ShouldMoveToDLQ: false,
				ErrorCode:       code,
				Description:     "Mailbox full",
			}

		// 553 - Invalid mailbox name (permanent)
		case "553":
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryInvalidRecipient,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       code,
				Description:     "Invalid mailbox name",
			}

		// 554 - Transaction failed (usually permanent)
		case "554":
			if strings.Contains(errorLower, "spam") || strings.Contains(errorLower, "blacklist") {
				return ErrorClassification{
					IsTemporary:     false,
					Category:        CategorySpam,
					RetryAfter:      0,
					ShouldMoveToDLQ: true,
					ErrorCode:       code,
					Description:     "Rejected as spam",
				}
			}
			if strings.Contains(errorLower, "block") {
				return ErrorClassification{
					IsTemporary:     false,
					Category:        CategoryBlocked,
					RetryAfter:      0,
					ShouldMoveToDLQ: true,
					ErrorCode:       code,
					Description:     "Sender blocked",
				}
			}
			return ErrorClassification{
				IsTemporary:     false,
				Category:        CategoryPermanent,
				RetryAfter:      0,
				ShouldMoveToDLQ: true,
				ErrorCode:       code,
				Description:     "Transaction failed",
			}

		default:
			// Generic 4xx = temporary, 5xx = permanent
			if code[0] == '4' {
				return ErrorClassification{
					IsTemporary:     true,
					Category:        CategoryTemporary,
					RetryAfter:      5 * time.Minute,
					ShouldMoveToDLQ: false,
					ErrorCode:       code,
					Description:     "Temporary failure",
				}
			} else if code[0] == '5' {
				return ErrorClassification{
					IsTemporary:     false,
					Category:        CategoryPermanent,
					RetryAfter:      0,
					ShouldMoveToDLQ: true,
					ErrorCode:       code,
					Description:     "Permanent failure",
				}
			}
		}
	}

	// 3. Keyword-based classification (fallback)
	if strings.Contains(errorLower, "timeout") || strings.Contains(errorLower, "connection") {
		return ErrorClassification{
			IsTemporary:     true,
			Category:        CategoryNetworkError,
			RetryAfter:      30 * time.Second,
			ShouldMoveToDLQ: false,
			ErrorCode:       "network_error",
			Description:     "Network error",
		}
	}

	if strings.Contains(errorLower, "authentication") || strings.Contains(errorLower, "auth") {
		return ErrorClassification{
			IsTemporary:     false,
			Category:        CategoryAuthError,
			RetryAfter:      0,
			ShouldMoveToDLQ: true,
			ErrorCode:       "auth_error",
			Description:     "Authentication failed",
		}
	}

	if strings.Contains(errorLower, "rate") || strings.Contains(errorLower, "throttle") {
		return ErrorClassification{
			IsTemporary:     true,
			Category:        CategoryRateLimit,
			RetryAfter:      15 * time.Minute,
			ShouldMoveToDLQ: false,
			ErrorCode:       "rate_limit",
			Description:     "Rate limit exceeded",
		}
	}

	// 4. Default: treat as unknown temporary error (safe default)
	return ErrorClassification{
		IsTemporary:     true,
		Category:        CategoryUnknown,
		RetryAfter:      5 * time.Minute,
		ShouldMoveToDLQ: false,
		ErrorCode:       "unknown",
		Description:     "Unknown error",
	}
}
