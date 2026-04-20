package email

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/rs/zerolog"
)

// DeliverabilityValidator validates SPF and DKIM records before sending.
type DeliverabilityValidator struct {
	logger zerolog.Logger
}

// NewDeliverabilityValidator creates a new deliverability validator.
func NewDeliverabilityValidator(logger zerolog.Logger) *DeliverabilityValidator {
	return &DeliverabilityValidator{
		logger: logger,
	}
}

// ValidationResult contains the results of deliverability validation.
type ValidationResult struct {
	Valid      bool
	SPFValid   bool
	DKIMValid  bool
	Warnings   []string
	Errors     []string
	SPFRecord  string
	DKIMRecord string
}

// ValidateDeliverability validates SPF and DKIM records for a domain.
func (dv *DeliverabilityValidator) ValidateDeliverability(ctx context.Context, domain string, senderIP string) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Warnings: []string{},
		Errors:   []string{},
	}

	// Validate SPF
	spfValid, spfRecord, spfWarnings, spfErrors := dv.validateSPF(ctx, domain, senderIP)
	result.SPFValid = spfValid
	result.SPFRecord = spfRecord
	result.Warnings = append(result.Warnings, spfWarnings...)
	result.Errors = append(result.Errors, spfErrors...)

	// Validate DKIM
	dkimValid, dkimRecord, dkimWarnings, dkimErrors := dv.validateDKIM(ctx, domain)
	result.DKIMValid = dkimValid
	result.DKIMRecord = dkimRecord
	result.Warnings = append(result.Warnings, dkimWarnings...)
	result.Errors = append(result.Errors, dkimErrors...)

	// Overall validity (at least one should be valid for basic deliverability)
	result.Valid = result.SPFValid || result.DKIMValid

	if !result.Valid {
		result.Errors = append(result.Errors, "neither SPF nor DKIM validation passed - email may be rejected")
	}

	return result
}

// validateSPF validates SPF record for the domain.
func (dv *DeliverabilityValidator) validateSPF(ctx context.Context, domain string, senderIP string) (bool, string, []string, []string) {
	var warnings []string
	var errors []string

	// Look up SPF record
	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		errors = append(errors, fmt.Sprintf("failed to lookup TXT records for %s: %v", domain, err))
		return false, "", warnings, errors
	}

	var spfRecord string
	for _, record := range txtRecords {
		if strings.HasPrefix(record, "v=spf1") {
			spfRecord = record
			break
		}
	}

	if spfRecord == "" {
		errors = append(errors, fmt.Sprintf("no SPF record found for domain %s", domain))
		return false, "", warnings, errors
	}

	// Basic SPF validation
	if !strings.Contains(spfRecord, "include:") && !strings.Contains(spfRecord, "a") && !strings.Contains(spfRecord, "mx") && !strings.Contains(spfRecord, "ip4:") && !strings.Contains(spfRecord, "ip6:") {
		warnings = append(warnings, "SPF record may be too restrictive - no include, a, mx, or ip mechanisms found")
	}

	// Check if SPF record allows the sender IP (basic check)
	if senderIP != "" {
		if strings.Contains(spfRecord, fmt.Sprintf("ip4:%s", senderIP)) {
			dv.logger.Debug().Str("domain", domain).Str("ip", senderIP).Msg("sender IP explicitly allowed in SPF")
		} else if strings.Contains(spfRecord, "include:") {
			warnings = append(warnings, "sender IP not explicitly listed - relying on include mechanisms")
		} else {
			warnings = append(warnings, "sender IP may not be authorized by SPF record")
		}
	}

	// Check for common SPF issues
	if strings.Count(spfRecord, "include:") > 10 {
		warnings = append(warnings, "SPF record has many includes - may exceed DNS lookup limit")
	}

	if !strings.HasSuffix(spfRecord, " ~all") && !strings.HasSuffix(spfRecord, " -all") {
		warnings = append(warnings, "SPF record should end with ~all or -all")
	}

	return true, spfRecord, warnings, errors
}

// validateDKIM validates DKIM record for the domain.
func (dv *DeliverabilityValidator) validateDKIM(ctx context.Context, domain string) (bool, string, []string, []string) {
	var warnings []string
	var errors []string

	// Common DKIM selectors to check
	selectors := []string{"default", "mail", "dkim", "k1", "selector1", "selector2", "s1", "s2"}

	var dkimRecord string
	var foundSelector string

	for _, selector := range selectors {
		dkimDomain := fmt.Sprintf("%s._domainkey.%s", selector, domain)

		txtRecords, err := net.LookupTXT(dkimDomain)
		if err != nil {
			continue // Try next selector
		}

		for _, record := range txtRecords {
			if strings.Contains(record, "k=rsa") || strings.Contains(record, "k=ed25519") {
				dkimRecord = record
				foundSelector = selector
				break
			}
		}

		if dkimRecord != "" {
			break
		}
	}

	if dkimRecord == "" {
		errors = append(errors, fmt.Sprintf("no DKIM record found for domain %s (checked selectors: %s)", domain, strings.Join(selectors, ", ")))
		return false, "", warnings, errors
	}

	// Basic DKIM validation
	if !strings.Contains(dkimRecord, "p=") {
		errors = append(errors, "DKIM record missing public key (p= parameter)")
		return false, dkimRecord, warnings, errors
	}

	if strings.Contains(dkimRecord, "p=;") || strings.Contains(dkimRecord, "p=\"\"") {
		errors = append(errors, "DKIM record has empty public key - key may be revoked")
		return false, dkimRecord, warnings, errors
	}

	// Check key algorithm
	if strings.Contains(dkimRecord, "k=rsa") {
		// RSA key - check if it's strong enough
		if !strings.Contains(dkimRecord, "h=sha256") && strings.Contains(dkimRecord, "h=") {
			warnings = append(warnings, "DKIM record should use SHA-256 hash algorithm")
		}
	} else if strings.Contains(dkimRecord, "k=ed25519") {
		dv.logger.Debug().Str("domain", domain).Msg("using Ed25519 DKIM key (modern)")
	} else {
		warnings = append(warnings, "DKIM key algorithm not specified or unknown")
	}

	// Check service type
	if strings.Contains(dkimRecord, "s=email") || strings.Contains(dkimRecord, "s=*") {
		// Good - allows email service
	} else if strings.Contains(dkimRecord, "s=") {
		warnings = append(warnings, "DKIM record may restrict service types")
	}

	dv.logger.Debug().
		Str("domain", domain).
		Str("selector", foundSelector).
		Msg("DKIM record found and validated")

	return true, dkimRecord, warnings, errors
}

// ValidateQuick performs a quick validation without detailed checks.
func (dv *DeliverabilityValidator) ValidateQuick(ctx context.Context, domain string) (bool, error) {
	// Quick SPF check
	txtRecords, err := net.LookupTXT(domain)
	if err == nil {
		for _, record := range txtRecords {
			if strings.HasPrefix(record, "v=spf1") {
				return true, nil // Found SPF record
			}
		}
	}

	// Quick DKIM check (just check for default selector)
	dkimDomain := fmt.Sprintf("default._domainkey.%s", domain)
	_, err = net.LookupTXT(dkimDomain)
	if err == nil {
		return true, nil // Found DKIM record
	}

	return false, fmt.Errorf("no SPF or DKIM records found for domain %s", domain)
}

// GetRecommendations provides recommendations for improving deliverability.
func (dv *DeliverabilityValidator) GetRecommendations(result *ValidationResult) []string {
	var recommendations []string

	if !result.SPFValid {
		recommendations = append(recommendations, "Set up SPF record: v=spf1 include:_spf.swiftmail.io ~all")
	}

	if !result.DKIMValid {
		recommendations = append(recommendations, "Set up DKIM record with a 2048-bit RSA key or Ed25519 key")
	}

	if result.SPFValid && result.DKIMValid {
		recommendations = append(recommendations, "Consider setting up DMARC policy: v=DMARC1; p=quarantine; rua=mailto:dmarc@yourdomain.com")
	}

	if len(result.Warnings) > 0 {
		recommendations = append(recommendations, "Review and address the warnings to improve deliverability")
	}

	return recommendations
}
