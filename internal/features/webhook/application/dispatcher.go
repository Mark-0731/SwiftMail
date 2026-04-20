package application

import (
webhook "github.com/Mark-0731/SwiftMail/internal/features/webhook"
	"github.com/Mark-0731/SwiftMail/internal/features/webhook/infrastructure"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Dispatcher handles webhook delivery with retries and HMAC signing.
type Dispatcher struct {
	repo   *infrastructure.Repository
	client *http.Client
	logger zerolog.Logger
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(repo *infrastructure.Repository, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		repo: repo,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Dispatch sends a webhook event to all matching endpoints for a user.
func (d *Dispatcher) Dispatch(ctx context.Context, userID uuid.UUID, eventType string, data map[string]interface{}) {
	webhooks, err := d.repo.GetByUserID(ctx, userID)
	if err != nil {
		d.logger.Error().Err(err).Msg("failed to get webhooks")
		return
	}

	for _, wh := range webhooks {
		if !wh.Active || !containsEvent(wh.Events, eventType) {
			continue
		}

		go d.deliverWithRetry(ctx, &wh, eventType, data)
	}
}

func (d *Dispatcher) deliverWithRetry(ctx context.Context, wh *webhook.Config, eventType string, data map[string]interface{}) {
	payload := map[string]interface{}{
		"event":     eventType,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(payload)

	backoffs := []time.Duration{0, 5 * time.Second, 30 * time.Second}

	for attempt, backoff := range backoffs {
		if backoff > 0 {
			time.Sleep(backoff)
		}

		signature := signPayload(body, wh.Secret)

		req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-SwiftMail-Signature", signature)
		req.Header.Set("X-SwiftMail-Event", eventType)

		resp, err := d.client.Do(req)
		if err != nil {
			d.logger.Warn().Err(err).Int("attempt", attempt+1).Str("url", wh.URL).Msg("webhook delivery failed")
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			d.logger.Debug().Str("event", eventType).Str("url", wh.URL).Msg("webhook delivered")
			return
		}

		d.logger.Warn().Int("status", resp.StatusCode).Int("attempt", attempt+1).Msg("webhook non-2xx response")
	}

	d.logger.Error().Str("url", wh.URL).Str("event", eventType).Msg("webhook delivery failed after all retries")
}

func signPayload(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func containsEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}
