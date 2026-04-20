package smtprelay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// AuthPlain implements PLAIN authentication using API keys.
func (s *Session) AuthPlain(username, password string) error {
	ctx := context.Background()

	// Username is the API key - hash it for lookup
	keyHash := s.backend.apiKeyMgr.HashAPIKey(username)

	// Try to get from cache first
	cached, err := s.backend.apiKeyMgr.GetCachedAPIKey(ctx, keyHash)
	if err == nil && cached != "" {
		// Parse cached data to get user ID
		var data struct {
			UserID string `json:"user_id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(cached), &data); err == nil {
			if data.Status == "active" {
				userID, _ := uuid.Parse(data.UserID)
				s.userID = userID
				s.authenticated = true
				s.logger.Info().Str("user_id", userID.String()).Msg("SMTP authentication successful (cached)")
				return nil
			}
		}
	}

	// Not in cache, validate from database
	apiKey, err := s.backend.authRepo.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		s.logger.Warn().Str("username", username[:12]).Msg("SMTP authentication failed")
		return fmt.Errorf("authentication failed")
	}

	// Check if key is expired
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		s.logger.Warn().Str("key_id", apiKey.ID.String()).Msg("API key expired")
		return fmt.Errorf("API key expired")
	}

	// Update last used timestamp
	s.backend.authRepo.UpdateAPIKeyLastUsed(ctx, apiKey.ID)

	s.userID = apiKey.UserID
	s.authenticated = true
	s.logger.Info().Str("user_id", apiKey.UserID.String()).Msg("SMTP authentication successful")
	return nil
}

// Mail sets the sender address.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if !s.authenticated {
		return fmt.Errorf("authentication required")
	}
	s.from = from
	s.logger.Debug().Str("from", from).Msg("MAIL FROM")
	return nil
}

// Rcpt adds a recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if !s.authenticated {
		return fmt.Errorf("authentication required")
	}
	s.to = append(s.to, to)
	s.logger.Debug().Str("to", to).Msg("RCPT TO")
	return nil
}

// Data receives the email body and queues it.
func (s *Session) Data(r io.Reader) error {
	if !s.authenticated {
		return fmt.Errorf("authentication required")
	}

	// Read the entire message
	msgBytes, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}

	ctx := context.Background()

	// Parse basic headers (simplified - in production use a proper MIME parser)
	msg := string(msgBytes)
	subject := extractHeader(msg, "Subject")

	// Queue each recipient as a separate job
	for _, recipient := range s.to {
		emailLogID := uuid.New()
		messageID := fmt.Sprintf("<%s@swiftmail>", uuid.New().String())

		// Create email log entry
		_, err := s.backend.db.Exec(ctx,
			`INSERT INTO email_logs (id, user_id, message_id, from_email, to_email, subject, status)
			 VALUES ($1, $2, $3, $4, $5, $6, 'queued')`,
			emailLogID, s.userID, messageID, s.from, recipient, subject,
		)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to create email log")
			return fmt.Errorf("failed to queue email")
		}

		// Create Asynq task
		payload := map[string]interface{}{
			"email_log_id": emailLogID.String(),
			"from":         s.from,
			"to":           recipient,
			"subject":      subject,
			"html":         "", // Would parse from MIME
			"text":         msg,
			"message_id":   messageID,
			"user_id":      s.userID.String(),
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		task := asynq.NewTask("email:send", payloadBytes,
			asynq.Queue("high"),
			asynq.MaxRetry(5),
			asynq.TaskID(emailLogID.String()),
		)

		if _, err := s.backend.asynqClient.EnqueueContext(ctx, task); err != nil {
			s.logger.Error().Err(err).Msg("failed to enqueue email")
			return fmt.Errorf("failed to queue email")
		}

		s.logger.Info().
			Str("email_log_id", emailLogID.String()).
			Str("from", s.from).
			Str("to", recipient).
			Msg("email queued via SMTP")
	}

	return nil
}

// Reset resets the session state.
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Logout ends the session.
func (s *Session) Logout() error {
	return nil
}

// extractHeader extracts a header value from a raw email message.
func extractHeader(msg, header string) string {
	lines := strings.Split(msg, "\n")
	prefix := header + ":"
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
