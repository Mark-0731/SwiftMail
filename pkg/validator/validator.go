package validator

import (
	"net/mail"
	"regexp"
	"strings"
)

var (
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	uuidRegex   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
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
