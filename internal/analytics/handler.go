package analytics

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/Mark-0731/SwiftMail/internal/server/middleware"
	"github.com/Mark-0731/SwiftMail/pkg/response"
)

// Handler holds analytics HTTP handlers.
type Handler struct {
	service *Service
}

// NewHandler creates analytics handlers.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Overview returns aggregated stats.
func (h *Handler) Overview(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	from, to := parseTimeRange(c)

	overview, err := h.service.GetOverview(c.Context(), userID, from, to)
	if err != nil {
		return response.InternalError(c, "Failed to get analytics overview")
	}

	return response.OK(c, overview)
}

// TimeSeries returns daily data points.
func (h *Handler) TimeSeries(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	from, to := parseTimeRange(c)

	points, err := h.service.GetTimeSeries(c.Context(), userID, from, to)
	if err != nil {
		return response.InternalError(c, "Failed to get time series")
	}

	return response.OK(c, points)
}

// TopDomains returns top recipient domains.
func (h *Handler) TopDomains(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	from, to := parseTimeRange(c)
	limit := c.QueryInt("limit", 10)

	domains, err := h.service.GetTopDomains(c.Context(), userID, from, to, limit)
	if err != nil {
		return response.InternalError(c, "Failed to get domain stats")
	}

	return response.OK(c, domains)
}

func parseTimeRange(c *fiber.Ctx) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7)
	to := now

	if f := c.Query("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := c.Query("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed.Add(24*time.Hour - time.Second)
		}
	}

	return from, to
}
