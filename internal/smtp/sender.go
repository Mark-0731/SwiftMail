package smtp

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/swiftmail/swiftmail/pkg/dkim"
	"github.com/swiftmail/swiftmail/pkg/mailer"
	"github.com/swiftmail/swiftmail/pkg/metrics"
)

// SendRequest is the input for SMTP sending.
type SendRequest struct {
	From         string
	To           string
	Subject      string
	HTML         string
	Text         string
	ReplyTo      string
	Headers      map[string]string
	MessageID    string
	DKIMSelector string // DKIM selector for signing
	DKIMDomain   string // Domain for DKIM signature
	DKIMKey      []byte // PEM-encoded private key
}

// Sender orchestrates email delivery through the connection pool with circuit breaker and failover.
type Sender struct {
	pool           *Pool
	circuitBreaker *CircuitBreaker
	mxResolver     *MXResolver
	metrics        *metrics.Metrics
	logger         zerolog.Logger
}

// NewSender creates a new SMTP sender.
func NewSender(pool *Pool, cb *CircuitBreaker, mx *MXResolver, m *metrics.Metrics, logger zerolog.Logger) *Sender {
	return &Sender{
		pool:           pool,
		circuitBreaker: cb,
		mxResolver:     mx,
		metrics:        m,
		logger:         logger,
	}
}

// Send delivers an email via the SMTP connection pool.
func (s *Sender) Send(ctx context.Context, req *SendRequest) (string, error) {
	recipientDomain := isRecipientDomain(req.To)

	// Check circuit breaker for destination domain
	allowed, err := s.circuitBreaker.AllowSend(ctx, recipientDomain)
	if err != nil {
		s.logger.Warn().Err(err).Str("domain", recipientDomain).Msg("circuit breaker check failed")
	}
	if !allowed {
		return "", fmt.Errorf("circuit breaker open for domain %s", recipientDomain)
	}

	// Compose MIME message
	msg := &mailer.Message{
		From:      req.From,
		To:        req.To,
		Subject:   req.Subject,
		HTMLBody:  req.HTML,
		TextBody:  req.Text,
		ReplyTo:   req.ReplyTo,
		MessageID: req.MessageID,
		Headers:   req.Headers,
	}

	mimeBytes, err := mailer.Compose(msg)
	if err != nil {
		return "", fmt.Errorf("failed to compose MIME message: %w", err)
	}

	// Apply DKIM signature if credentials provided
	if req.DKIMSelector != "" && req.DKIMDomain != "" && len(req.DKIMKey) > 0 {
		privateKey, err := dkim.ParsePrivateKey(req.DKIMKey)
		if err != nil {
			s.logger.Warn().Err(err).Msg("failed to parse DKIM key, sending without signature")
		} else {
			// Extract headers for DKIM signing
			headers := map[string]string{
				"from":       req.From,
				"to":         req.To,
				"subject":    req.Subject,
				"message-id": req.MessageID,
			}

			dkimHeader, err := dkim.Sign(privateKey, req.DKIMDomain, req.DKIMSelector, headers, req.HTML)
			if err != nil {
				s.logger.Warn().Err(err).Msg("DKIM signing failed, sending without signature")
			} else {
				// Prepend DKIM-Signature header to message
				mimeBytes = append([]byte(dkimHeader+"\r\n"), mimeBytes...)
			}
		}
	}

	// Get connection from pool
	conn, err := s.pool.Get()
	if err != nil {
		return "", fmt.Errorf("failed to get SMTP connection: %w", err)
	}

	// Send
	sendErr := conn.SendMail(req.From, req.To, mimeBytes)

	if sendErr != nil {
		// Return bad connection, don't put back in pool
		conn.Close()
		s.circuitBreaker.RecordFailure(ctx, recipientDomain)

		s.metrics.EmailsSentTotal.WithLabelValues("error", recipientDomain).Inc()

		smtpCode := extractSMTPCode(sendErr)
		return smtpCode + " " + sendErr.Error(), sendErr
	}

	// Success — return connection to pool
	s.pool.Put(conn)
	s.circuitBreaker.RecordSuccess(ctx, recipientDomain)
	s.metrics.SMTPPoolReuseTotal.Inc()

	return "250 OK", nil
}
