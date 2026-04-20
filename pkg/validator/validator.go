package validator

import (
	"net"
	"net/mail"
	"regexp"
	"strings"
)

var (
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	uuidRegex   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// RFC 5322 compliant email regex
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

	// Disposable email domains (common ones)
	disposableDomains = map[string]bool{
		"tempmail.com":      true,
		"guerrillamail.com": true,
		"10minutemail.com":  true,
		"mailinator.com":    true,
		"throwaway.email":   true,
		"temp-mail.org":     true,
		"fakeinbox.com":     true,
		"trashmail.com":     true,
	}

	// Role-based email prefixes
	roleBasedPrefixes = map[string]bool{
		"admin":         true,
		"administrator": true,
		"postmaster":    true,
		"hostmaster":    true,
		"webmaster":     true,
		"noreply":       true,
		"no-reply":      true,
		"support":       true,
		"info":          true,
		"sales":         true,
		"marketing":     true,
	}
)

// IsValidEmail checks if the given string is a valid email address.
func IsValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// IsValidDomain checks if the given string is a valid domain name.
func IsValidDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if len(domain) > 253 {
		return false
	}
	return domainRegex.MatchString(domain)
}

// IsValidUUID checks if the given string is a valid UUID v4.
func IsValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// NormalizeDomain lowercases and trims a domain name.
func NormalizeDomain(domain string) string {
	return strings.TrimSpace(strings.ToLower(domain))
}

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

// IsValidFilename checks if a filename is safe (no path traversal, reasonable length).
func IsValidFilename(filename string) bool {
	if len(filename) == 0 || len(filename) > 255 {
		return false
	}
	// Reject path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return false
	}
	// Reject control characters
	for _, r := range filename {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// EmailValidationResult contains detailed validation results.
type EmailValidationResult struct {
	Valid        bool
	Email        string
	Reason       string
	IsDisposable bool
	IsRoleBased  bool
	HasValidMX   bool
	Domain       string
}

// ValidateEmailAdvanced performs comprehensive email validation.
func ValidateEmailAdvanced(email string, checkMX bool) *EmailValidationResult {
	result := &EmailValidationResult{
		Email: email,
		Valid: true,
	}

	email = strings.TrimSpace(strings.ToLower(email))

	// 1. Basic format check
	if !IsValidEmail(email) {
		result.Valid = false
		result.Reason = "invalid email format"
		return result
	}

	// 2. Extract domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		result.Valid = false
		result.Reason = "invalid email structure"
		return result
	}

	localPart := parts[0]
	domain := parts[1]
	result.Domain = domain

	// 3. Check local part length
	if len(localPart) > 64 {
		result.Valid = false
		result.Reason = "local part too long (max 64 characters)"
		return result
	}

	// 4. Check for disposable email domains
	if disposableDomains[domain] {
		result.IsDisposable = true
		result.Valid = false
		result.Reason = "disposable email address not allowed"
		return result
	}

	// 5. Check for role-based emails
	if roleBasedPrefixes[localPart] {
		result.IsRoleBased = true
		// Note: We don't mark as invalid, just flag it
	}

	// 6. Check MX records (optional, can be slow)
	if checkMX {
		mxRecords, err := net.LookupMX(domain)
		if err != nil || len(mxRecords) == 0 {
			result.HasValidMX = false
			result.Valid = false
			result.Reason = "domain has no valid MX records"
			return result
		}
		result.HasValidMX = true
	}

	return result
}

// IsDisposableEmail checks if an email is from a disposable domain.
func IsDisposableEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	return disposableDomains[parts[1]]
}

// IsRoleBasedEmail checks if an email is role-based.
func IsRoleBasedEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	return roleBasedPrefixes[parts[0]]
}

// AddDisposableDomain adds a domain to the disposable list.
func AddDisposableDomain(domain string) {
	disposableDomains[strings.ToLower(domain)] = true
}

// RemoveDisposableDomain removes a domain from the disposable list.
func RemoveDisposableDomain(domain string) {
	delete(disposableDomains, strings.ToLower(domain))
}
