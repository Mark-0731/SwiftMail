package application

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Schedule defines the IP warmup ramp (14 days to fully warmed).
var Schedule = []struct {
	Day   int
	Limit int
}{
	{1, 50}, {2, 100}, {3, 200}, {4, 400},
	{5, 800}, {6, 1500}, {7, 3000}, {8, 5000},
	{9, 8000}, {10, 12000}, {11, 18000}, {12, 25000},
	{13, 35000}, {14, 50000},
}

// ISPDistribution defines the recommended volume distribution across major ISPs.
var ISPDistribution = map[string]float64{
	"gmail.com":   0.40,
	"outlook.com": 0.20,
	"yahoo.com":   0.15,
	"hotmail.com": 0.10,
	"other":       0.15,
}

// Scheduler manages IP warmup progression.
type Scheduler struct {
	db     *pgxpool.Pool
	logger zerolog.Logger
}

// NewScheduler creates a warmup scheduler.
func NewScheduler(db *pgxpool.Pool, logger zerolog.Logger) *Scheduler {
	return &Scheduler{db: db, logger: logger}
}

// GetDailyLimit returns the daily sending limit for an IP based on its warmup day.
func (s *Scheduler) GetDailyLimit(warmupDay int) int {
	if warmupDay <= 0 {
		return Schedule[0].Limit
	}
	if warmupDay > len(Schedule) {
		return Schedule[len(Schedule)-1].Limit // Fully warmed
	}
	return Schedule[warmupDay-1].Limit
}

// AdvanceWarmup moves IPs to the next warmup day if conditions are met.
// Should be called daily via cron job.
func (s *Scheduler) AdvanceWarmup(ctx context.Context) error {
	// Only advance IPs with bounce rate < 2% and complaint rate < 0.05%
	rows, err := s.db.Query(ctx,
		`SELECT id, ip, warmup_day FROM ip_addresses WHERE warmup_active = TRUE AND warmup_day < 14`,
	)
	if err != nil {
		return fmt.Errorf("failed to query warming IPs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, ip string
		var day int
		if err := rows.Scan(&id, &ip, &day); err != nil {
			continue
		}

		newDay := day + 1
		newLimit := s.GetDailyLimit(newDay)

		_, err := s.db.Exec(ctx,
			`UPDATE ip_addresses SET warmup_day = $1, daily_limit = $2, daily_sent = 0 WHERE id = $3`,
			newDay, newLimit, id,
		)
		if err != nil {
			s.logger.Error().Err(err).Str("ip", ip).Msg("failed to advance warmup")
			continue
		}

		if newDay >= 14 {
			s.db.Exec(ctx, `UPDATE ip_addresses SET warmup_active = FALSE WHERE id = $1`, id)
			s.logger.Info().Str("ip", ip).Msg("IP fully warmed up")
		} else {
			s.logger.Info().Str("ip", ip).Int("day", newDay).Int("limit", newLimit).Msg("warmup advanced")
		}
	}

	return nil
}

// GetIPProgress returns the warmup progress for a specific IP.
func (s *Scheduler) GetIPProgress(ctx context.Context, ip string) (day int, limit int, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT warmup_day, daily_limit FROM ip_addresses WHERE ip = $1`, ip,
	).Scan(&day, &limit)
	return day, limit, err
}
