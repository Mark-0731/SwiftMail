package provider

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// SendGridProvider implements Provider using SendGrid API.
type SendGridProvider struct {
	apiKey string
	client *sendgrid.Client
	logger zerolog.Logger
}

// NewSendGridProvider creates a new SendGrid provider.
func NewSendGridProvider(apiKey string, logger zerolog.Logger) Provider {
	return &SendGridProvider{
		apiKey: apiKey,
		client: sendgrid.NewSendClient(apiKey),
		logger: logger,
	}
}

// Name returns the provider name.
func (p *SendGridProvider) Name() string {
	return "sendgrid"
}

// Send sends an email via SendGrid API.
func (p *SendGridProvider) Send(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	// Parse from email
	from := mail.NewEmail("", req.From)

	// Parse to email
	to := mail.NewEmail("", req.To)

	// Create message
	message := mail.NewSingleEmail(from, req.Subject, to, req.Text, req.HTML)

	// Set custom message ID header
	if req.MessageID != "" {
		message.SetHeader("Message-ID", req.MessageID)
	}

	// Set reply-to
	if req.ReplyTo != nil && *req.ReplyTo != "" {
		replyTo := mail.NewEmail("", *req.ReplyTo)
		message.SetReplyTo(replyTo)
	}

	// Add custom headers
	for key, value := range req.Headers {
		message.SetHeader(key, value)
	}

	// Add attachments
	for _, att := range req.Attachments {
		attachment := mail.NewAttachment()
		attachment.SetFilename(att.Filename)
		attachment.SetType(att.ContentType)
		attachment.SetContent(base64.StdEncoding.EncodeToString(att.Data))
		attachment.SetDisposition("attachment")
		message.AddAttachment(attachment)
	}

	// Send via SendGrid
	response, err := p.client.SendWithContext(ctx, message)
	if err != nil {
		p.logger.Error().Err(err).Str("to", req.To).Msg("SendGrid send failed")
		return &SendResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	// Check response status
	if response.StatusCode >= 400 {
		errMsg := fmt.Sprintf("SendGrid returned status %d: %s", response.StatusCode, response.Body)
		p.logger.Error().
			Int("status_code", response.StatusCode).
			Str("body", response.Body).
			Str("to", req.To).
			Msg("SendGrid send failed")
		return &SendResponse{
			Success: false,
			Error:   errMsg,
		}, fmt.Errorf("%s", errMsg)
	}

	// Extract message ID from response headers
	messageID := req.MessageID
	if xMessageID, ok := response.Headers["X-Message-Id"]; ok && len(xMessageID) > 0 {
		messageID = xMessageID[0]
	}

	p.logger.Info().
		Str("to", req.To).
		Str("message_id", messageID).
		Int("status_code", response.StatusCode).
		Msg("email sent via SendGrid")

	return &SendResponse{
		ProviderMessageID: messageID,
		Success:           true,
	}, nil
}
