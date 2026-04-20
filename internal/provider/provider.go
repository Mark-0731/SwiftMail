package provider

import (
	"context"
)

// Provider defines the interface for email sending providers.
type Provider interface {
	Send(ctx context.Context, req *SendRequest) (*SendResponse, error)
	Name() string
}

// SendRequest represents an email send request.
type SendRequest struct {
	From        string
	To          string
	Subject     string
	HTML        string
	Text        string
	ReplyTo     *string
	Headers     map[string]string
	MessageID   string
	Attachments []Attachment
}

// Attachment represents an email attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// SendResponse represents the provider's response.
type SendResponse struct {
	ProviderMessageID string
	Success           bool
	Error             string
}
