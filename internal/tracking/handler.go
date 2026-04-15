package tracking

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/swiftmail/swiftmail/internal/worker"
)

// 1x1 transparent PNG pixel (pre-computed, served from memory).
var pixelPNG []byte

func init() {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Transparent)
	var buf bytes.Buffer
	png.Encode(&buf, img)
	pixelPNG = buf.Bytes()
}

// Handler holds tracking HTTP handlers.
type Handler struct {
	asynqClient *asynq.Client
	rdb         *redis.Client
	logger      zerolog.Logger
}

// NewHandler creates tracking handlers.
func NewHandler(asynqClient *asynq.Client, rdb *redis.Client, logger zerolog.Logger) *Handler {
	return &Handler{
		asynqClient: asynqClient,
		rdb:         rdb,
		logger:      logger,
	}
}

// OpenPixel serves the 1x1 tracking pixel and records the open event.
// Target: < 20ms response time.
func (h *Handler) OpenPixel(c *fiber.Ctx) error {
	id := c.Params("id")
	emailLogID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(404).Send(pixelPNG)
	}

	// Fire-and-forget tracking event (async)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error().Interface("panic", r).Msg("panic in tracking goroutine")
			}
		}()

		payload := worker.TrackingEventPayload{
			EmailLogID: emailLogID,
			EventType:  "opened",
			IPAddress:  c.IP(),
			UserAgent:  c.Get("User-Agent"),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to marshal tracking payload")
			return
		}
		task := asynq.NewTask(worker.TaskTrackingEvent, data, asynq.Queue("low"))
		if _, err := h.asynqClient.Enqueue(task); err != nil {
			h.logger.Error().Err(err).Msg("failed to enqueue tracking event")
		}
	}()

	// Return pixel immediately (< 20ms)
	c.Set("Content-Type", "image/png")
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Set("Pragma", "no-cache")
	return c.Send(pixelPNG)
}

// ClickRedirect handles click tracking and redirects to the original URL.
// Target: < 30ms response time.
func (h *Handler) ClickRedirect(c *fiber.Ctx) error {
	id := c.Params("id")
	emailLogID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).SendString("Invalid tracking ID")
	}

	url := c.Query("url")
	if url == "" {
		return c.Status(400).SendString("Missing URL")
	}

	// Fire-and-forget tracking event (async)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error().Interface("panic", r).Msg("panic in tracking goroutine")
			}
		}()

		payload := worker.TrackingEventPayload{
			EmailLogID: emailLogID,
			EventType:  "clicked",
			IPAddress:  c.IP(),
			UserAgent:  c.Get("User-Agent"),
			URL:        url,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to marshal tracking payload")
			return
		}
		task := asynq.NewTask(worker.TaskTrackingEvent, data, asynq.Queue("low"))
		if _, err := h.asynqClient.Enqueue(task); err != nil {
			h.logger.Error().Err(err).Msg("failed to enqueue tracking event")
		}
	}()

	// Redirect immediately (< 30ms)
	return c.Redirect(url, 302)
}

// Unsubscribe handles one-click unsubscribe.
func (h *Handler) Unsubscribe(c *fiber.Ctx) error {
	token := c.Params("token")

	// Decode token to get email and user_id
	// Token format: base64(email:user_id:timestamp:signature)
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return c.Status(400).SendString("Invalid unsubscribe token")
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 {
		return c.Status(400).SendString("Invalid token format")
	}

	email := parts[0]
	userID := parts[1]

	// Add to suppression list in Redis
	emailHash := hashEmail(email)
	ctx := context.Background()

	err = h.rdb.SAdd(ctx, fmt.Sprintf("suppress:%s", userID), emailHash).Err()
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to add to suppression list")
		return c.Status(500).SendString("Failed to process unsubscribe")
	}

	h.logger.Info().Str("email", email).Str("user_id", userID).Msg("user unsubscribed")

	return c.SendString("You have been unsubscribed successfully.")
}

func hashEmail(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}
