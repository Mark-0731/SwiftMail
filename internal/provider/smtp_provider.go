package provider

import (
	"context"

	"github.com/Mark-0731/SwiftMail/internal/smtp"
	"github.com/rs/zerolog"
)

// SMTPProvider implements Provider using your own SMTP infrastructure.
type SMTPProvider struct {
	sender *smtp.Sender
	logger zerolog.Logger
}

// NewSMTPProvider creates a new SMTP provider.
func NewSMTPProvider(sender *smtp.Sender, logger zerolog.Logger) Provider {
	return &SMTPProvider{
		sender: sender,
		logger: logger,
	}
}

// Name returns the provider name.
func (p *SMTPProvider) Name() string {
	return "smtp"
}

// Send sends an email via SMTP.
func (p *SMTPProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	// Convert ReplyTo pointer to string
	replyTo := ""
	if req.ReplyTo != nil {
		replyTo = *req.ReplyTo
	}

	// Convert to SMTP sender format
	smtpReq := &smtp.SendRequest{
		From:      req.From,
		To:        req.To,
		Subject:   req.Subject,
		HTML:      req.HTML,
		Text:      req.Text,
		ReplyTo:   replyTo,
		Headers:   req.Headers,
		MessageID: req.MessageID,
	}

	// Note: SMTP sender doesn't support attachments yet
	// Attachments are handled at the MIME level in the actual SMTP implementation

	// Send via SMTP
	smtpResponse, err := p.sender.Send(ctx, smtpReq)
	if err != nil {
		p.logger.Error().Err(err).Str("to", req.To).Msg("SMTP send failed")
		return &SendResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	p.logger.Info().
		Str("to", req.To).
		Str("message_id", req.MessageID).
		Str("smtp_response", smtpResponse).
		Msg("email sent via SMTP")

	return &SendResponse{
		ProviderMessageID: req.MessageID, // Use our message ID
		Success:           true,
	}, nil
}
