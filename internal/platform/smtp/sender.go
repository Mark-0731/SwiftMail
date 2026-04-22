package smtp

import (
	"context"
	"fmt"
	"time"

	"github.com/Mark-0731/SwiftMail/pkg/dkim"
	"github.com/Mark-0731/SwiftMail/pkg/mailer"
	"github.com/Mark-0731/SwiftMail/pkg/metrics"
	"github.com/rs/zerolog"
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
	startTime := time.Now()
	recipientDomain := isRecipientDomain(req.To)

	// Check circuit breaker for destination domain
	cbStart := time.Now()
	allowed, err := s.circuitBreaker.AllowSend(ctx, recipientDomain)
	cbDuration := time.Since(cbStart).Milliseconds()
	if err != nil {
		s.logger.Warn().Err(err).Str("domain", recipientDomain).Msg("circuit breaker check failed")
	}
	if !allowed {
		return "", fmt.Errorf("circuit breaker open for domain %s", recipientDomain)
	}

	// Compose MIME message
	composeStart := time.Now()
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
	composeDuration := time.Since(composeStart).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("failed to compose MIME message: %w", err)
	}

	// Apply DKIM signature if credentials provided
	dkimStart := time.Now()
	dkimDuration := int64(0)
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
		dkimDuration = time.Since(dkimStart).Milliseconds()
	}

	// Get connection from pool
	poolGetStart := time.Now()
	conn, err := s.pool.Get()
	poolGetDuration := time.Since(poolGetStart).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("failed to get SMTP connection: %w", err)
	}

	// Send
	smtpSendStart := time.Now()
	sendErr := conn.SendMail(req.From, req.To, mimeBytes)
	smtpSendDuration := time.Since(smtpSendStart).Milliseconds()

	totalDuration := time.Since(startTime).Milliseconds()

	if sendErr != nil {
		// Return bad connection, don't put back in pool
		conn.Close()
		s.circuitBreaker.RecordFailure(ctx, recipientDomain)

		s.metrics.EmailsSentTotal.WithLabelValues("error", recipientDomain).Inc()

		s.logger.Error().
			Str("to", req.To).
			Int64("total_ms", totalDuration).
			Int64("circuit_breaker_ms", cbDuration).
			Int64("compose_ms", composeDuration).
			Int64("dkim_ms", dkimDuration).
			Int64("pool_get_ms", poolGetDuration).
			Int64("smtp_send_ms", smtpSendDuration).
			Err(sendErr).
			Msg("SMTP send timing breakdown (failed)")

		smtpCode := extractSMTPCode(sendErr)
		return smtpCode + " " + sendErr.Error(), sendErr
	}

	// Success — return connection to pool
	s.pool.Put(conn)
	s.circuitBreaker.RecordSuccess(ctx, recipientDomain)
	s.metrics.SMTPPoolReuseTotal.Inc()

	s.logger.Info().
		Str("to", req.To).
		Int64("total_ms", totalDuration).
		Int64("circuit_breaker_ms", cbDuration).
		Int64("compose_ms", composeDuration).
		Int64("dkim_ms", dkimDuration).
		Int64("pool_get_ms", poolGetDuration).
		Int64("smtp_send_ms", smtpSendDuration).
		Msg("SMTP send timing breakdown (success)")

	return "250 OK", nil
}
