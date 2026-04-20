package pipeline

import (
	"context"
	"fmt"

	emailservice "github.com/Mark-0731/SwiftMail/internal/email/service"
	"github.com/rs/zerolog"
)

// SecurityStage handles spam detection, content sanitization, and attachment validation.
type SecurityStage struct {
	spamDetector        *emailservice.SpamDetector
	sanitizer           *emailservice.ContentSanitizer
	attachmentValidator *emailservice.AttachmentValidator
	deliverability      *emailservice.DeliverabilityValidator
	logger              zerolog.Logger
}

// NewSecurityStage creates a new security stage.
func NewSecurityStage(
	spamDetector *emailservice.SpamDetector,
	sanitizer *emailservice.ContentSanitizer,
	attachmentValidator *emailservice.AttachmentValidator,
	deliverability *emailservice.DeliverabilityValidator,
	logger zerolog.Logger,
) Stage {
	return &SecurityStage{
		spamDetector:        spamDetector,
		sanitizer:           sanitizer,
		attachmentValidator: attachmentValidator,
		deliverability:      deliverability,
		logger:              logger,
	}
}

// Name returns the stage name.
func (s *SecurityStage) Name() string {
	return "security"
}

// Execute performs security checks.
func (s *SecurityStage) Execute(ctx context.Context, state *State) error {
	// Use rendered content if available, otherwise use original
	subject := state.RenderedSubject
	if subject == "" {
		subject = state.Subject
	}
	htmlBody := state.RenderedHTML
	if htmlBody == "" {
		htmlBody = state.HTML
	}
	textBody := state.RenderedText
	if textBody == "" {
		textBody = state.Text
	}

	// 1. Content sanitization
	sanitizedSubject, sanitizedHTML, sanitizedText, sanitizedHeaders := s.sanitizer.SanitizeAll(
		subject,
		htmlBody,
		textBody,
		state.Headers,
	)

	state.RenderedSubject = sanitizedSubject
	state.RenderedHTML = sanitizedHTML
	state.RenderedText = sanitizedText
	state.SanitizedHeaders = sanitizedHeaders

	s.logger.Debug().
		Str("user_id", state.UserID.String()).
		Bool("html_sanitized", sanitizedHTML != htmlBody).
		Bool("text_sanitized", sanitizedText != textBody).
		Bool("subject_sanitized", sanitizedSubject != subject).
		Msg("content sanitization completed")

	// 2. Spam detection
	spamScore := s.spamDetector.AnalyzeContent(sanitizedSubject, sanitizedHTML, sanitizedText)
	state.SpamScore = spamScore.Score

	if spamScore.IsSpam {
		s.logger.Warn().
			Str("user_id", state.UserID.String()).
			Int("spam_score", spamScore.Score).
			Strs("reasons", spamScore.Reasons).
			Msg("email flagged as spam")
		return fmt.Errorf("email content flagged as spam (score: %d/100)", spamScore.Score)
	}

	if spamScore.Score > 40 {
		s.logger.Warn().
			Str("user_id", state.UserID.String()).
			Int("spam_score", spamScore.Score).
			Strs("reasons", spamScore.Reasons).
			Msg("email has elevated spam score")
	}

	// 3. Attachment validation
	if len(state.Attachments) > 0 {
		serviceAttachments := make([]emailservice.AttachmentData, len(state.Attachments))
		for i, att := range state.Attachments {
			serviceAttachments[i] = emailservice.AttachmentData{
				Filename:    att.Filename,
				ContentType: att.ContentType,
				Data:        att.Data,
				Size:        att.Size,
			}
		}

		attachmentResult := s.attachmentValidator.ValidateAttachments(serviceAttachments)
		if !attachmentResult.Valid {
			s.logger.Warn().
				Str("user_id", state.UserID.String()).
				Str("reason", attachmentResult.Reason).
				Strs("invalid_attachments", attachmentResult.InvalidAttachments).
				Msg("attachment validation failed")
			return fmt.Errorf("attachment validation failed: %s", attachmentResult.Reason)
		}

		s.logger.Info().
			Str("user_id", state.UserID.String()).
			Int("attachment_count", attachmentResult.AttachmentCount).
			Int64("total_size_bytes", attachmentResult.TotalSize).
			Msg("attachments validated successfully")
	}

	// 4. Email size validation
	totalEmailSize := int64(len(sanitizedSubject) + len(sanitizedHTML) + len(sanitizedText))
	for _, attachment := range state.Attachments {
		totalEmailSize += attachment.Size
	}

	const MaxEmailSize = 25 * 1024 * 1024 // 25MB
	if totalEmailSize > MaxEmailSize {
		s.logger.Warn().
			Str("user_id", state.UserID.String()).
			Int64("email_size_bytes", totalEmailSize).
			Int64("max_size_bytes", MaxEmailSize).
			Msg("email size exceeds SMTP limits")
		return fmt.Errorf("email size (%d MB) exceeds maximum allowed size (%d MB)",
			totalEmailSize/(1024*1024), MaxEmailSize/(1024*1024))
	}

	// 5. Deliverability validation
	fromDomain := extractDomain(state.From)
	if fromDomain != "" {
		deliverabilityValid, err := s.deliverability.ValidateQuick(ctx, fromDomain)
		if err != nil {
			s.logger.Warn().
				Str("user_id", state.UserID.String()).
				Str("domain", fromDomain).
				Err(err).
				Msg("deliverability validation failed - email may have poor deliverability")
		} else if !deliverabilityValid {
			s.logger.Warn().
				Str("user_id", state.UserID.String()).
				Str("domain", fromDomain).
				Msg("no SPF or DKIM records found - email may be rejected by recipients")
		} else {
			s.logger.Debug().
				Str("user_id", state.UserID.String()).
				Str("domain", fromDomain).
				Msg("deliverability validation passed")
		}
	}

	return nil
}

func extractDomain(email string) string {
	parts := splitEmail(email)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func splitEmail(email string) []string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return []string{email[:i], email[i+1:]}
		}
	}
	return []string{email}
}
