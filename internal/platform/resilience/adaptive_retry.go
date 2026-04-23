package resilience

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/Mark-0731/SwiftMail/pkg/database"
)

// RetryStrategy represents a learned retry strategy for a domain/error type
type RetryStrategy struct {
	Domain           string
	ErrorCategory    string
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	SuccessRate      float64
	AverageRetries   float64
	LastUpdated      time.Time
	SampleSize       int
	OptimalTimeSlots []int // Hours of day when success rate is highest
}

// AdaptiveRetryEngine learns from failure patterns and adjusts retry timing
type AdaptiveRetryEngine struct {
	db         database.Querier
	logger     zerolog.Logger
	strategies map[string]*RetryStrategy // key: "domain:error_category"
	mu         sync.RWMutex

	// Configuration
	minSampleSize      int     // Minimum samples before adapting
	learningRate       float64 // How quickly to adapt (0-1)
	defaultBaseDelay   time.Duration
	defaultMaxDelay    time.Duration
	adaptationInterval time.Duration // How often to recalculate strategies
}

// NewAdaptiveRetryEngine creates a new adaptive retry engine
func NewAdaptiveRetryEngine(db database.Querier, logger zerolog.Logger) *AdaptiveRetryEngine {
	engine := &AdaptiveRetryEngine{
		db:                 db,
		logger:             logger,
		strategies:         make(map[string]*RetryStrategy),
		minSampleSize:      10,
		learningRate:       0.3,
		defaultBaseDelay:   5 * time.Minute,
		defaultMaxDelay:    2 * time.Hour,
		adaptationInterval: 15 * time.Minute,
	}

	// Load existing strategies from database
	engine.loadStrategies(context.Background())

	return engine
}

// GetRetryDelay calculates the optimal retry delay based on learned patterns
func (e *AdaptiveRetryEngine) GetRetryDelay(
	domain, errorCategory string,
	retryCount int,
) time.Duration {
	strategy := e.getStrategy(domain, errorCategory)

	// Base exponential backoff
	baseDelay := strategy.BaseDelay
	delay := time.Duration(math.Pow(2, float64(retryCount))) * baseDelay

	// Cap at max delay
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	// Apply jitter (±20%)
	jitter := time.Duration(rand.Float64()*0.4-0.2) * delay
	delay += jitter

	// Time-of-day optimization
	if len(strategy.OptimalTimeSlots) > 0 {
		currentHour := time.Now().Hour()
		isOptimalTime := false
		for _, hour := range strategy.OptimalTimeSlots {
			if currentHour == hour {
				isOptimalTime = true
				break
			}
		}

		// If not optimal time, add delay to shift to optimal time
		if !isOptimalTime {
			nextOptimalHour := e.getNextOptimalHour(currentHour, strategy.OptimalTimeSlots)
			hoursUntilOptimal := (nextOptimalHour - currentHour + 24) % 24

			// Add up to 2 hours to shift towards optimal time
			if hoursUntilOptimal > 0 && hoursUntilOptimal <= 2 {
				delay += time.Duration(hoursUntilOptimal) * time.Hour
			}
		}
	}

	// Adjust based on success rate
	if strategy.SuccessRate < 0.3 {
		// Low success rate, increase delay
		delay = time.Duration(float64(delay) * 1.5)
	} else if strategy.SuccessRate > 0.7 {
		// High success rate, can be more aggressive
		delay = time.Duration(float64(delay) * 0.8)
	}

	e.logger.Debug().
		Str("domain", domain).
		Str("error_category", errorCategory).
		Int("retry_count", retryCount).
		Dur("calculated_delay", delay).
		Float64("success_rate", strategy.SuccessRate).
		Msg("calculated adaptive retry delay")

	return delay
}

// RecordRetryOutcome records the outcome of a retry attempt
func (e *AdaptiveRetryEngine) RecordRetryOutcome(
	ctx context.Context,
	domain, errorCategory string,
	retryCount int,
	success bool,
	duration time.Duration,
) {
	key := e.strategyKey(domain, errorCategory)

	e.mu.Lock()
	strategy, exists := e.strategies[key]
	if !exists {
		strategy = &RetryStrategy{
			Domain:        domain,
			ErrorCategory: errorCategory,
			BaseDelay:     e.defaultBaseDelay,
			MaxDelay:      e.defaultMaxDelay,
			LastUpdated:   time.Now(),
		}
		e.strategies[key] = strategy
	}
	e.mu.Unlock()

	// Update metrics
	e.mu.Lock()
	strategy.SampleSize++

	// Update success rate with exponential moving average
	if success {
		strategy.SuccessRate = strategy.SuccessRate*(1-e.learningRate) + e.learningRate
	} else {
		strategy.SuccessRate = strategy.SuccessRate * (1 - e.learningRate)
	}

	// Update average retries
	strategy.AverageRetries = strategy.AverageRetries*(1-e.learningRate) +
		float64(retryCount)*e.learningRate

	strategy.LastUpdated = time.Now()
	e.mu.Unlock()

	// Adapt strategy if we have enough samples
	if strategy.SampleSize >= e.minSampleSize {
		e.adaptStrategy(strategy)
	}

	// Persist to database asynchronously
	go e.persistStrategy(ctx, strategy)
}

// adaptStrategy adjusts the retry strategy based on learned patterns
func (e *AdaptiveRetryEngine) adaptStrategy(strategy *RetryStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Adjust base delay based on success rate
	if strategy.SuccessRate < 0.3 {
		// Low success rate, increase base delay
		newBaseDelay := time.Duration(float64(strategy.BaseDelay) * 1.2)
		if newBaseDelay <= strategy.MaxDelay/4 {
			strategy.BaseDelay = newBaseDelay
		}
	} else if strategy.SuccessRate > 0.7 && strategy.AverageRetries < 2 {
		// High success rate with few retries, can be more aggressive
		newBaseDelay := time.Duration(float64(strategy.BaseDelay) * 0.9)
		if newBaseDelay >= 1*time.Minute {
			strategy.BaseDelay = newBaseDelay
		}
	}

	// Adjust max delay based on average retries
	if strategy.AverageRetries > 3 {
		// Many retries needed, increase max delay
		newMaxDelay := time.Duration(float64(strategy.MaxDelay) * 1.1)
		if newMaxDelay <= 6*time.Hour {
			strategy.MaxDelay = newMaxDelay
		}
	}

	e.logger.Info().
		Str("domain", strategy.Domain).
		Str("error_category", strategy.ErrorCategory).
		Dur("base_delay", strategy.BaseDelay).
		Dur("max_delay", strategy.MaxDelay).
		Float64("success_rate", strategy.SuccessRate).
		Float64("avg_retries", strategy.AverageRetries).
		Int("sample_size", strategy.SampleSize).
		Msg("adapted retry strategy")
}

// getStrategy returns the retry strategy for a domain/error combination
func (e *AdaptiveRetryEngine) getStrategy(domain, errorCategory string) *RetryStrategy {
	key := e.strategyKey(domain, errorCategory)

	e.mu.RLock()
	strategy, exists := e.strategies[key]
	e.mu.RUnlock()

	if exists {
		return strategy
	}

	// Return default strategy
	return &RetryStrategy{
		Domain:        domain,
		ErrorCategory: errorCategory,
		BaseDelay:     e.defaultBaseDelay,
		MaxDelay:      e.defaultMaxDelay,
		SuccessRate:   0.5, // Neutral assumption
	}
}

// strategyKey generates a unique key for a strategy
func (e *AdaptiveRetryEngine) strategyKey(domain, errorCategory string) string {
	return domain + ":" + errorCategory
}

// getNextOptimalHour finds the next optimal hour from current hour
func (e *AdaptiveRetryEngine) getNextOptimalHour(currentHour int, optimalHours []int) int {
	minDistance := 24
	nextHour := currentHour

	for _, hour := range optimalHours {
		distance := (hour - currentHour + 24) % 24
		if distance > 0 && distance < minDistance {
			minDistance = distance
			nextHour = hour
		}
	}

	return nextHour
}

// loadStrategies loads retry strategies from database
func (e *AdaptiveRetryEngine) loadStrategies(ctx context.Context) {
	if e.db == nil {
		return
	}

	rows, err := e.db.Query(ctx, `
		SELECT domain, error_category, base_delay_ms, max_delay_ms, 
		       success_rate, average_retries, sample_size, optimal_time_slots
		FROM adaptive_retry_strategies
		WHERE last_updated > NOW() - INTERVAL '7 days'
	`)
	if err != nil {
		e.logger.Error().Err(err).Msg("failed to load retry strategies")
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var strategy RetryStrategy
		var baseDelayMs, maxDelayMs int64
		var optimalSlots []int

		err := rows.Scan(
			&strategy.Domain,
			&strategy.ErrorCategory,
			&baseDelayMs,
			&maxDelayMs,
			&strategy.SuccessRate,
			&strategy.AverageRetries,
			&strategy.SampleSize,
			&optimalSlots,
		)
		if err != nil {
			e.logger.Error().Err(err).Msg("failed to scan retry strategy")
			continue
		}

		strategy.BaseDelay = time.Duration(baseDelayMs) * time.Millisecond
		strategy.MaxDelay = time.Duration(maxDelayMs) * time.Millisecond
		strategy.OptimalTimeSlots = optimalSlots

		key := e.strategyKey(strategy.Domain, strategy.ErrorCategory)
		e.strategies[key] = &strategy
		count++
	}

	e.logger.Info().
		Int("count", count).
		Msg("loaded adaptive retry strategies from database")
}

// persistStrategy saves a retry strategy to database
func (e *AdaptiveRetryEngine) persistStrategy(ctx context.Context, strategy *RetryStrategy) {
	if e.db == nil {
		return
	}

	baseDelayMs := strategy.BaseDelay.Milliseconds()
	maxDelayMs := strategy.MaxDelay.Milliseconds()

	_, err := e.db.Exec(ctx, `
		INSERT INTO adaptive_retry_strategies (
			domain, error_category, base_delay_ms, max_delay_ms,
			success_rate, average_retries, sample_size, optimal_time_slots,
			last_updated, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (domain, error_category)
		DO UPDATE SET
			base_delay_ms = EXCLUDED.base_delay_ms,
			max_delay_ms = EXCLUDED.max_delay_ms,
			success_rate = EXCLUDED.success_rate,
			average_retries = EXCLUDED.average_retries,
			sample_size = EXCLUDED.sample_size,
			optimal_time_slots = EXCLUDED.optimal_time_slots,
			last_updated = NOW()
	`, strategy.Domain, strategy.ErrorCategory, baseDelayMs, maxDelayMs,
		strategy.SuccessRate, strategy.AverageRetries, strategy.SampleSize,
		strategy.OptimalTimeSlots)

	if err != nil {
		e.logger.Error().Err(err).Msg("failed to persist retry strategy")
	}
}

// StartAdaptationWorker starts a background worker to periodically adapt strategies
func (e *AdaptiveRetryEngine) StartAdaptationWorker(ctx context.Context) {
	ticker := time.NewTicker(e.adaptationInterval)
	defer ticker.Stop()

	e.logger.Info().
		Dur("interval", e.adaptationInterval).
		Msg("adaptive retry engine worker started")

	for {
		select {
		case <-ticker.C:
			e.analyzeAndAdapt(ctx)
		case <-ctx.Done():
			e.logger.Info().Msg("adaptive retry engine worker stopped")
			return
		}
	}
}

// analyzeAndAdapt analyzes recent retry patterns and adapts strategies
func (e *AdaptiveRetryEngine) analyzeAndAdapt(ctx context.Context) {
	if e.db == nil {
		return
	}

	// Analyze retry patterns from DLQ retry history
	rows, err := e.db.Query(ctx, `
		SELECT 
			dlq.recipient_domain,
			dlq.error_code,
			COUNT(*) as total_retries,
			SUM(CASE WHEN rh.retry_result = 'success' THEN 1 ELSE 0 END) as successful_retries,
			AVG(rh.retry_attempt) as avg_retry_attempt,
			EXTRACT(HOUR FROM rh.retried_at) as retry_hour
		FROM dlq_retry_history rh
		JOIN dead_letter_queue dlq ON rh.dlq_entry_id = dlq.id
		WHERE rh.retried_at > NOW() - INTERVAL '24 hours'
		GROUP BY dlq.recipient_domain, dlq.error_code, EXTRACT(HOUR FROM rh.retried_at)
		HAVING COUNT(*) >= 5
	`)
	if err != nil {
		e.logger.Error().Err(err).Msg("failed to analyze retry patterns")
		return
	}
	defer rows.Close()

	adaptedCount := 0
	for rows.Next() {
		var domain, errorCode string
		var totalRetries, successfulRetries int
		var avgRetryAttempt float64
		var retryHour int

		err := rows.Scan(&domain, &errorCode, &totalRetries, &successfulRetries,
			&avgRetryAttempt, &retryHour)
		if err != nil {
			continue
		}

		successRate := float64(successfulRetries) / float64(totalRetries)

		// Update or create strategy
		key := e.strategyKey(domain, errorCode)
		e.mu.Lock()
		strategy, exists := e.strategies[key]
		if !exists {
			strategy = &RetryStrategy{
				Domain:        domain,
				ErrorCategory: errorCode,
				BaseDelay:     e.defaultBaseDelay,
				MaxDelay:      e.defaultMaxDelay,
			}
			e.strategies[key] = strategy
		}

		// Update metrics
		strategy.SuccessRate = successRate
		strategy.AverageRetries = avgRetryAttempt
		strategy.SampleSize += totalRetries

		// Track optimal time slots
		if successRate > 0.7 {
			found := false
			for _, hour := range strategy.OptimalTimeSlots {
				if hour == retryHour {
					found = true
					break
				}
			}
			if !found && len(strategy.OptimalTimeSlots) < 6 {
				strategy.OptimalTimeSlots = append(strategy.OptimalTimeSlots, retryHour)
			}
		}

		e.mu.Unlock()

		// Adapt the strategy
		e.adaptStrategy(strategy)
		adaptedCount++
	}

	if adaptedCount > 0 {
		e.logger.Info().
			Int("adapted_count", adaptedCount).
			Msg("adapted retry strategies based on recent patterns")
	}
}

// GetAllStrategies returns all current retry strategies
func (e *AdaptiveRetryEngine) GetAllStrategies() map[string]*RetryStrategy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	strategies := make(map[string]*RetryStrategy)
	for key, strategy := range e.strategies {
		// Return a copy to prevent external modification
		strategyCopy := *strategy
		strategies[key] = &strategyCopy
	}

	return strategies
}

// ResetStrategy resets a specific retry strategy to defaults
func (e *AdaptiveRetryEngine) ResetStrategy(domain, errorCategory string) {
	key := e.strategyKey(domain, errorCategory)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.strategies[key] = &RetryStrategy{
		Domain:        domain,
		ErrorCategory: errorCategory,
		BaseDelay:     e.defaultBaseDelay,
		MaxDelay:      e.defaultMaxDelay,
		SuccessRate:   0.5,
		LastUpdated:   time.Now(),
	}

	e.logger.Info().
		Str("domain", domain).
		Str("error_category", errorCategory).
		Msg("reset retry strategy to defaults")
}
