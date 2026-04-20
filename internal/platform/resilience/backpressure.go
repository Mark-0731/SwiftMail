package resilience

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// BackpressureController manages system load and prevents overload
type BackpressureController struct {
	logger zerolog.Logger

	// Configuration
	maxQueueDepth      int64
	criticalQueueDepth int64
	maxConcurrency     int32
	minConcurrency     int32
	adjustmentInterval time.Duration

	// State
	currentConcurrency atomic.Int32
	currentQueueDepth  atomic.Int64
	rejectedCount      atomic.Int64
	processedCount     atomic.Int64
	lastAdjustment     time.Time
	mu                 sync.RWMutex

	// Metrics
	avgProcessingTime time.Duration
	successRate       float64

	// Control
	enabled atomic.Bool
	paused  atomic.Bool
}

// NewBackpressureController creates a new backpressure controller
func NewBackpressureController(
	maxConcurrency, minConcurrency int,
	maxQueueDepth int64,
	logger zerolog.Logger,
) *BackpressureController {
	bc := &BackpressureController{
		logger:             logger,
		maxConcurrency:     int32(maxConcurrency),
		minConcurrency:     int32(minConcurrency),
		maxQueueDepth:      maxQueueDepth,
		criticalQueueDepth: int64(float64(maxQueueDepth) * 0.8), // 80% threshold
		adjustmentInterval: 30 * time.Second,
		lastAdjustment:     time.Now(),
		successRate:        1.0,
	}

	bc.currentConcurrency.Store(int32(maxConcurrency))
	bc.enabled.Store(true)

	return bc
}

// ShouldAccept determines if a new task should be accepted
func (bc *BackpressureController) ShouldAccept(ctx context.Context) bool {
	if !bc.enabled.Load() {
		return true // Backpressure disabled
	}

	if bc.paused.Load() {
		bc.rejectedCount.Add(1)
		bc.logger.Warn().Msg("backpressure: system paused, rejecting task")
		return false
	}

	queueDepth := bc.currentQueueDepth.Load()

	// Critical threshold - reject all new tasks
	if queueDepth >= bc.maxQueueDepth {
		bc.rejectedCount.Add(1)
		bc.logger.Error().
			Int64("queue_depth", queueDepth).
			Int64("max_depth", bc.maxQueueDepth).
			Msg("backpressure: queue at max capacity, rejecting task")
		return false
	}

	// Warning threshold - probabilistic rejection
	if queueDepth >= bc.criticalQueueDepth {
		// Reject with probability based on how close to max
		rejectProbability := float64(queueDepth-bc.criticalQueueDepth) /
			float64(bc.maxQueueDepth-bc.criticalQueueDepth)

		if shouldReject(rejectProbability) {
			bc.rejectedCount.Add(1)
			bc.logger.Warn().
				Int64("queue_depth", queueDepth).
				Float64("reject_probability", rejectProbability).
				Msg("backpressure: probabilistic rejection")
			return false
		}
	}

	return true
}

// RecordTaskStart records that a task has started processing
func (bc *BackpressureController) RecordTaskStart() {
	bc.currentQueueDepth.Add(-1)
}

// RecordTaskComplete records that a task has completed
func (bc *BackpressureController) RecordTaskComplete(duration time.Duration, success bool) {
	bc.processedCount.Add(1)

	bc.mu.Lock()
	// Update average processing time (exponential moving average)
	if bc.avgProcessingTime == 0 {
		bc.avgProcessingTime = duration
	} else {
		alpha := 0.2 // Smoothing factor
		bc.avgProcessingTime = time.Duration(
			float64(bc.avgProcessingTime)*(1-alpha) + float64(duration)*alpha,
		)
	}

	// Update success rate
	if success {
		bc.successRate = bc.successRate*0.95 + 0.05 // 95% old, 5% new success
	} else {
		bc.successRate = bc.successRate * 0.95 // Decay on failure
	}
	bc.mu.Unlock()
}

// RecordTaskEnqueued records that a task has been enqueued
func (bc *BackpressureController) RecordTaskEnqueued() {
	bc.currentQueueDepth.Add(1)
}

// GetCurrentConcurrency returns the current concurrency limit
func (bc *BackpressureController) GetCurrentConcurrency() int {
	return int(bc.currentConcurrency.Load())
}

// AdjustConcurrency dynamically adjusts concurrency based on system metrics
func (bc *BackpressureController) AdjustConcurrency() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Don't adjust too frequently
	if time.Since(bc.lastAdjustment) < bc.adjustmentInterval {
		return
	}

	currentConcurrency := bc.currentConcurrency.Load()
	queueDepth := bc.currentQueueDepth.Load()
	successRate := bc.successRate

	var newConcurrency int32
	var reason string

	// Decision logic
	if queueDepth > bc.criticalQueueDepth {
		// Queue building up - reduce concurrency
		newConcurrency = int32(float64(currentConcurrency) * 0.8)
		reason = "high_queue_depth"
	} else if successRate < 0.7 {
		// Low success rate - reduce concurrency
		newConcurrency = int32(float64(currentConcurrency) * 0.9)
		reason = "low_success_rate"
	} else if queueDepth < bc.criticalQueueDepth/2 && successRate > 0.9 {
		// System healthy - can increase concurrency
		newConcurrency = int32(float64(currentConcurrency) * 1.1)
		reason = "system_healthy"
	} else {
		// No change needed
		bc.lastAdjustment = time.Now()
		return
	}

	// Apply bounds
	if newConcurrency < bc.minConcurrency {
		newConcurrency = bc.minConcurrency
	}
	if newConcurrency > bc.maxConcurrency {
		newConcurrency = bc.maxConcurrency
	}

	// Only update if changed
	if newConcurrency != currentConcurrency {
		bc.currentConcurrency.Store(newConcurrency)
		bc.lastAdjustment = time.Now()

		bc.logger.Info().
			Int32("old_concurrency", currentConcurrency).
			Int32("new_concurrency", newConcurrency).
			Int64("queue_depth", queueDepth).
			Float64("success_rate", successRate).
			Str("reason", reason).
			Msg("adjusted concurrency")
	}
}

// GetMetrics returns current backpressure metrics
func (bc *BackpressureController) GetMetrics() map[string]interface{} {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	processed := bc.processedCount.Load()
	rejected := bc.rejectedCount.Load()
	total := processed + rejected

	rejectionRate := 0.0
	if total > 0 {
		rejectionRate = float64(rejected) / float64(total)
	}

	return map[string]interface{}{
		"enabled":              bc.enabled.Load(),
		"paused":               bc.paused.Load(),
		"current_concurrency":  bc.currentConcurrency.Load(),
		"max_concurrency":      bc.maxConcurrency,
		"min_concurrency":      bc.minConcurrency,
		"queue_depth":          bc.currentQueueDepth.Load(),
		"max_queue_depth":      bc.maxQueueDepth,
		"critical_queue_depth": bc.criticalQueueDepth,
		"processed_count":      processed,
		"rejected_count":       rejected,
		"rejection_rate":       rejectionRate,
		"avg_processing_time":  bc.avgProcessingTime.String(),
		"success_rate":         bc.successRate,
	}
}

// Pause temporarily pauses task acceptance
func (bc *BackpressureController) Pause() {
	bc.paused.Store(true)
	bc.logger.Warn().Msg("backpressure: system paused")
}

// Resume resumes task acceptance
func (bc *BackpressureController) Resume() {
	bc.paused.Store(false)
	bc.logger.Info().Msg("backpressure: system resumed")
}

// Enable enables backpressure control
func (bc *BackpressureController) Enable() {
	bc.enabled.Store(true)
	bc.logger.Info().Msg("backpressure: enabled")
}

// Disable disables backpressure control
func (bc *BackpressureController) Disable() {
	bc.enabled.Store(false)
	bc.logger.Info().Msg("backpressure: disabled")
}

// Reset resets all counters and state
func (bc *BackpressureController) Reset() {
	bc.currentQueueDepth.Store(0)
	bc.rejectedCount.Store(0)
	bc.processedCount.Store(0)
	bc.currentConcurrency.Store(bc.maxConcurrency)

	bc.mu.Lock()
	bc.avgProcessingTime = 0
	bc.successRate = 1.0
	bc.lastAdjustment = time.Now()
	bc.mu.Unlock()

	bc.logger.Info().Msg("backpressure: reset")
}

// StartMonitor starts a background monitor that adjusts concurrency
func (bc *BackpressureController) StartMonitor(ctx context.Context) {
	ticker := time.NewTicker(bc.adjustmentInterval)
	defer ticker.Stop()

	bc.logger.Info().
		Dur("interval", bc.adjustmentInterval).
		Msg("backpressure monitor started")

	for {
		select {
		case <-ticker.C:
			bc.AdjustConcurrency()
			bc.logMetrics()
		case <-ctx.Done():
			bc.logger.Info().Msg("backpressure monitor stopped")
			return
		}
	}
}

// logMetrics logs current metrics
func (bc *BackpressureController) logMetrics() {
	metrics := bc.GetMetrics()

	bc.logger.Debug().
		Interface("metrics", metrics).
		Msg("backpressure metrics")
}

// shouldReject determines if a task should be rejected based on probability
func shouldReject(probability float64) bool {
	if probability <= 0 {
		return false
	}
	if probability >= 1 {
		return true
	}

	// Use a simple random check
	// In production, consider using a better random source
	return float64(time.Now().UnixNano()%1000)/1000.0 < probability
}

// TenantBackpressureController manages per-tenant backpressure
type TenantBackpressureController struct {
	controllers map[string]*BackpressureController
	mu          sync.RWMutex
	logger      zerolog.Logger

	// Default limits
	defaultMaxConcurrency int
	defaultMinConcurrency int
	defaultMaxQueueDepth  int64
}

// NewTenantBackpressureController creates a new tenant-aware controller
func NewTenantBackpressureController(
	maxConcurrency, minConcurrency int,
	maxQueueDepth int64,
	logger zerolog.Logger,
) *TenantBackpressureController {
	return &TenantBackpressureController{
		controllers:           make(map[string]*BackpressureController),
		logger:                logger,
		defaultMaxConcurrency: maxConcurrency,
		defaultMinConcurrency: minConcurrency,
		defaultMaxQueueDepth:  maxQueueDepth,
	}
}

// GetController returns or creates a controller for a tenant
func (tbc *TenantBackpressureController) GetController(tenantID string) *BackpressureController {
	tbc.mu.RLock()
	controller, exists := tbc.controllers[tenantID]
	tbc.mu.RUnlock()

	if exists {
		return controller
	}

	// Create new controller
	tbc.mu.Lock()
	defer tbc.mu.Unlock()

	// Double-check after acquiring write lock
	if controller, exists := tbc.controllers[tenantID]; exists {
		return controller
	}

	controller = NewBackpressureController(
		tbc.defaultMaxConcurrency,
		tbc.defaultMinConcurrency,
		tbc.defaultMaxQueueDepth,
		tbc.logger.With().Str("tenant_id", tenantID).Logger(),
	)

	tbc.controllers[tenantID] = controller

	return controller
}

// GetAllMetrics returns metrics for all tenants
func (tbc *TenantBackpressureController) GetAllMetrics() map[string]interface{} {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	metrics := make(map[string]interface{})
	for tenantID, controller := range tbc.controllers {
		metrics[tenantID] = controller.GetMetrics()
	}

	return metrics
}

// StartMonitorAll starts monitors for all tenant controllers
func (tbc *TenantBackpressureController) StartMonitorAll(ctx context.Context) {
	tbc.mu.RLock()
	controllers := make([]*BackpressureController, 0, len(tbc.controllers))
	for _, controller := range tbc.controllers {
		controllers = append(controllers, controller)
	}
	tbc.mu.RUnlock()

	// Start monitor for each controller
	for _, controller := range controllers {
		go controller.StartMonitor(ctx)
	}
}
