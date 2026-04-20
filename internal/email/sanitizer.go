package email

import (
	"html"
	"regexp"
	"strings"
)

// ContentSanitizer sanitizes email content to prevent XSS and other security issues.
type ContentSanitizer struct {
	// Regex patterns for dangerous content
	scriptPattern     *regexp.Regexp
	onEventPattern    *regexp.Regexp
	javascriptPattern *regexp.Regexp
	vbscriptPattern   *regexp.Regexp
	dataUrlPattern    *regexp.Regexp
}

// NewContentSanitizer creates a new content sanitizer.
func NewContentSanitizer() *ContentSanitizer {
	return &ContentSanitizer{
		scriptPattern:     regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
		onEventPattern:    regexp.MustCompile(`(?i)\s*on\w+\s*=`),
		javascriptPattern: regexp.MustCompile(`(?i)javascript:`),
		vbscriptPattern:   regexp.MustCompile(`(?i)vbscript:`),
		dataUrlPattern:    regexp.MustCompile(`(?i)data:\s*text/html`),
	}
}

// SanitizeHTML sanitizes HTML content by removing dangerous elements and attributes.
func (cs *ContentSanitizer) SanitizeHTML(htmlContent string) string {
	if htmlContent == "" {
		return htmlContent
	}

	// Remove script tags
	htmlContent = cs.scriptPattern.ReplaceAllString(htmlContent, "")

	// Remove event handlers (onclick, onload, etc.)
	htmlContent = cs.onEventPattern.ReplaceAllString(htmlContent, "")

	// Remove javascript: URLs
	htmlContent = cs.javascriptPattern.ReplaceAllString(htmlContent, "")

	// Remove vbscript: URLs
	htmlContent = cs.vbscriptPattern.ReplaceAllString(htmlContent, "")

	// Remove data URLs with HTML content
	htmlContent = cs.dataUrlPattern.ReplaceAllString(htmlContent, "data:text/plain")

	// Remove potentially dangerous tags
	dangerousTags := []string{
		"<iframe", "</iframe>",
		"<object", "</object>",
		"<embed", "</embed>",
		"<applet", "</applet>",
		"<form", "</form>",
		"<input", "</input>",
		"<button", "</button>",
		"<textarea", "</textarea>",
		"<select", "</select>",
		"<option", "</option>",
		"<meta", "</meta>",
		"<link", "</link>",
		"<base", "</base>",
	}

	for _, tag := range dangerousTags {
		htmlContent = strings.ReplaceAll(htmlContent, tag, "")
	}

	return htmlContent
}

// SanitizeText sanitizes plain text content.
func (cs *ContentSanitizer) SanitizeText(textContent string) string {
	if textContent == "" {
		return textContent
	}

	// HTML escape the text content to prevent any HTML injection
	return html.EscapeString(textContent)
}

// SanitizeSubject sanitizes email subject line.
func (cs *ContentSanitizer) SanitizeSubject(subject string) string {
	if subject == "" {
		return subject
	}

	// Remove control characters and non-printable characters
	subject = regexp.MustCompile(`[\x00-\x1F\x7F]`).ReplaceAllString(subject, "")

	// Remove excessive whitespace
	subject = regexp.MustCompile(`\s+`).ReplaceAllString(subject, " ")

	// Trim whitespace
	subject = strings.TrimSpace(subject)

	// Limit subject length (RFC 5322 recommends max 78 characters)
	if len(subject) > 78 {
		subject = subject[:75] + "..."
	}

	return subject
}

// SanitizeHeaders sanitizes email headers.
func (cs *ContentSanitizer) SanitizeHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return headers
	}

	sanitized := make(map[string]string)

	// List of allowed headers
	allowedHeaders := map[string]bool{
		"reply-to":         true,
		"x-priority":       true,
		"x-mailer":         true,
		"list-id":          true,
		"list-unsubscribe": true,
		"precedence":       true,
		"auto-submitted":   true,
	}

	for key, value := range headers {
		lowerKey := strings.ToLower(key)

		// Only allow safe headers
		if !allowedHeaders[lowerKey] {
			continue
		}

		// Remove control characters from header values
		value = regexp.MustCompile(`[\x00-\x1F\x7F]`).ReplaceAllString(value, "")

		// Remove excessive whitespace
		value = regexp.MustCompile(`\s+`).ReplaceAllString(value, " ")

		// Trim whitespace
		value = strings.TrimSpace(value)

		// Limit header value length
		if len(value) > 200 {
			value = value[:197] + "..."
		}

		if value != "" {
			sanitized[key] = value
		}
	}

	return sanitized
}

// ValidateEmailAddresses validates and sanitizes email addresses.
func (cs *ContentSanitizer) ValidateEmailAddresses(emails ...string) []string {
	var sanitized []string

	emailPattern := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	for _, email := range emails {
		// Trim whitespace
		email = strings.TrimSpace(email)

		// Convert to lowercase
		email = strings.ToLower(email)

		// Remove control characters
		email = regexp.MustCompile(`[\x00-\x1F\x7F]`).ReplaceAllString(email, "")

		// Validate format
		if emailPattern.MatchString(email) && len(email) <= 254 { // RFC 5321 limit
			sanitized = append(sanitized, email)
		}
	}

	return sanitized
}

// SanitizeAll sanitizes all email content components.
func (cs *ContentSanitizer) SanitizeAll(subject, htmlBody, textBody string, headers map[string]string) (string, string, string, map[string]string) {
	return cs.SanitizeSubject(subject),
		cs.SanitizeHTML(htmlBody),
		cs.SanitizeText(textBody),
		cs.SanitizeHeaders(headers)
}
