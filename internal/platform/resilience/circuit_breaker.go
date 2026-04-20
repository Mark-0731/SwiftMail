package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// CircuitState represents the state of a circuit breaker
type CircuitState string

const (
	StateClosed   CircuitState = "closed"    // Normal operation
	StateOpen     CircuitState = "open"      // Failing, reject requests
	StateHalfOpen CircuitState = "half_open" // Testing recovery
)

// CircuitBreakerConfig holds configuration for circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold  int           // Number of failures before opening
	SuccessThreshold  int           // Number of successes to close from half-open
	Timeout           time.Duration // How long to wait before half-open
	HalfOpenMaxCalls  int           // Max calls allowed in half-open state
	SlidingWindowSize int           // Number of recent calls to track
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:  5,
		SuccessThreshold:  2,
		Timeout:           30 * time.Second,
		HalfOpenMaxCalls:  3,
		SlidingWindowSize: 100,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	resourceType string // "provider" or "domain"
	resourceID   string // provider name or domain name
	config       CircuitBreakerConfig
	db           *pgxpool.Pool
	logger       zerolog.Logger

	mu                sync.RWMutex
	state             CircuitState
	failureCount      int
	successCount      int
	lastFailureTime   time.Time
	lastStateChange   time.Time
	halfOpenCallCount int
	recentCalls       []bool // true = success, false = failure
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(
	resourceType, resourceID string,
	config CircuitBreakerConfig,
	db *pgxpool.Pool,
	logger zerolog.Logger,
) *CircuitBreaker {
	cb := &CircuitBreaker{
		resourceType: resourceType,
		resourceID:   resourceID,
		config:       config,
		db:           db,
		logger: logger.With().
			Str("resource_type", resourceType).
			Str("resource_id", resourceID).
			Logger(),
		state:       StateClosed,
		recentCalls: make([]bool, 0, config.SlidingWindowSize),
	}

	// Load state from database
	cb.loadState(context.Background())

	return cb
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	// Check if we can proceed
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err == nil)

	return err
}

// beforeCall checks if the call should be allowed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Normal operation, allow call
		return nil

	case StateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastStateChange) >= cb.config.Timeout {
			// Transition to half-open
			cb.transitionTo(StateHalfOpen)
			cb.halfOpenCallCount = 0
			return nil
		}
		// Still open, reject call
		return fmt.Errorf("circuit breaker open for %s:%s", cb.resourceType, cb.resourceID)

	case StateHalfOpen:
		// Allow limited calls in half-open state
		if cb.halfOpenCallCount >= cb.config.HalfOpenMaxCalls {
			return fmt.Errorf("circuit breaker half-open, max calls reached for %s:%s",
				cb.resourceType, cb.resourceID)
		}
		cb.halfOpenCallCount++
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state: %s", cb.state)
	}
}

// afterCall records the result and updates state
func (cb *CircuitBreaker) afterCall(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Add to recent calls (sliding window)
	cb.recentCalls = append(cb.recentCalls, success)
	if len(cb.recentCalls) > cb.config.SlidingWindowSize {
		cb.recentCalls = cb.recentCalls[1:]
	}

	if success {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}

	// Persist state asynchronously
	go cb.persistState(context.Background())
}

// onSuccess handles successful call
func (cb *CircuitBreaker) onSuccess() {
	cb.successCount++

	switch cb.state {
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0

	case StateHalfOpen:
		// Check if we have enough successes to close
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.transitionTo(StateClosed)
			cb.failureCount = 0
			cb.successCount = 0
			cb.logger.Info().Msg("circuit breaker closed after recovery")
		}

	case StateOpen:
		// Should not happen, but reset if it does
		cb.logger.Warn().Msg("success call in open state, resetting")
		cb.transitionTo(StateClosed)
	}
}

// onFailure handles failed call
func (cb *CircuitBreaker) onFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		// Check if we should open
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.transitionTo(StateOpen)
			cb.logger.Warn().
				Int("failure_count", cb.failureCount).
				Msg("circuit breaker opened due to failures")
		}

	case StateHalfOpen:
		// Any failure in half-open goes back to open
		cb.transitionTo(StateOpen)
		cb.successCount = 0
		cb.logger.Warn().Msg("circuit breaker reopened after half-open failure")

	case StateOpen:
		// Already open, just increment counter
	}
}

// transitionTo changes the circuit breaker state
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	cb.logger.Info().
		Str("old_state", string(oldState)).
		Str("new_state", string(newState)).
		Msg("circuit breaker state transition")
}

// GetState returns the current state (thread-safe)
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetMetrics returns current metrics
func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Calculate success rate from recent calls
	successRate := 0.0
	if len(cb.recentCalls) > 0 {
		successCount := 0
		for _, success := range cb.recentCalls {
			if success {
				successCount++
			}
		}
		successRate = float64(successCount) / float64(len(cb.recentCalls))
	}

	return map[string]interface{}{
		"state":              string(cb.state),
		"failure_count":      cb.failureCount,
		"success_count":      cb.successCount,
		"success_rate":       successRate,
		"last_failure_time":  cb.lastFailureTime,
		"last_state_change":  cb.lastStateChange,
		"recent_calls_count": len(cb.recentCalls),
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(StateClosed)
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenCallCount = 0
	cb.recentCalls = make([]bool, 0, cb.config.SlidingWindowSize)

	cb.logger.Info().Msg("circuit breaker manually reset")

	go cb.persistState(context.Background())
}

// loadState loads circuit breaker state from database
func (cb *CircuitBreaker) loadState(ctx context.Context) {
	if cb.db == nil {
		return
	}

	var state string
	var failureCount, successCount int
	var lastFailureTime, openedAt time.Time

	err := cb.db.QueryRow(ctx, `
		SELECT state, failure_count, success_count, last_failure_at, opened_at
		FROM circuit_breaker_state
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY updated_at DESC
		LIMIT 1
	`, cb.resourceType, cb.resourceID).Scan(
		&state, &failureCount, &successCount, &lastFailureTime, &openedAt,
	)

	if err != nil {
		// No existing state, use defaults
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitState(state)
	cb.failureCount = failureCount
	cb.successCount = successCount
	cb.lastFailureTime = lastFailureTime

	// If state is open and timeout has elapsed, transition to half-open
	if cb.state == StateOpen && time.Since(openedAt) >= cb.config.Timeout {
		cb.transitionTo(StateHalfOpen)
	}

	cb.logger.Debug().
		Str("loaded_state", string(cb.state)).
		Int("failure_count", failureCount).
		Msg("circuit breaker state loaded from database")
}

// persistState saves circuit breaker state to database
func (cb *CircuitBreaker) persistState(ctx context.Context) {
	if cb.db == nil {
		return
	}

	cb.mu.RLock()
	state := cb.state
	failureCount := cb.failureCount
	successCount := cb.successCount
	lastFailureTime := cb.lastFailureTime
	lastStateChange := cb.lastStateChange
	cb.mu.RUnlock()

	var openedAt *time.Time
	var halfOpenAt *time.Time

	if state == StateOpen {
		openedAt = &lastStateChange
	} else if state == StateHalfOpen {
		halfOpenAt = &lastStateChange
	}

	_, err := cb.db.Exec(ctx, `
		INSERT INTO circuit_breaker_state (
			id, resource_type, resource_id, state, failure_count, success_count,
			last_failure_at, opened_at, half_open_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (resource_type, resource_id)
		DO UPDATE SET
			state = EXCLUDED.state,
			failure_count = EXCLUDED.failure_count,
			success_count = EXCLUDED.success_count,
			last_failure_at = EXCLUDED.last_failure_at,
			opened_at = EXCLUDED.opened_at,
			half_open_at = EXCLUDED.half_open_at,
			updated_at = NOW()
	`, uuid.New(), cb.resourceType, cb.resourceID, string(state),
		failureCount, successCount, lastFailureTime, openedAt, halfOpenAt)

	if err != nil {
		cb.logger.Error().Err(err).Msg("failed to persist circuit breaker state")
	}
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
	db       *pgxpool.Pool
	logger   zerolog.Logger
}

// NewCircuitBreakerManager creates a new manager
func NewCircuitBreakerManager(
	config CircuitBreakerConfig,
	db *pgxpool.Pool,
	logger zerolog.Logger,
) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
		db:       db,
		logger:   logger,
	}
}

// GetBreaker returns or creates a circuit breaker for a resource
func (m *CircuitBreakerManager) GetBreaker(resourceType, resourceID string) *CircuitBreaker {
	key := fmt.Sprintf("%s:%s", resourceType, resourceID)

	m.mu.RLock()
	breaker, exists := m.breakers[key]
	m.mu.RUnlock()

	if exists {
		return breaker
	}

	// Create new breaker
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if breaker, exists := m.breakers[key]; exists {
		return breaker
	}

	breaker = NewCircuitBreaker(resourceType, resourceID, m.config, m.db, m.logger)
	m.breakers[key] = breaker

	return breaker
}

// GetAllStates returns states of all circuit breakers
func (m *CircuitBreakerManager) GetAllStates() map[string]CircuitState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]CircuitState)
	for key, breaker := range m.breakers {
		states[key] = breaker.GetState()
	}

	return states
}

// ResetAll resets all circuit breakers
func (m *CircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, breaker := range m.breakers {
		breaker.Reset()
	}
}
