package migrator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// CleanupErrorType represents different types of cleanup errors
type CleanupErrorType int

const (
	// Retryable errors that might succeed on retry
	CleanupErrorTransient CleanupErrorType = iota
	// Permanent errors that won't succeed on retry
	CleanupErrorPermanent
	// Rate limiting errors that need backoff
	CleanupErrorRateLimit
	// Timeout errors
	CleanupErrorTimeout
	// Authentication/authorization errors
	CleanupErrorAuth
	// Resource not found errors
	CleanupErrorNotFound
)

// CleanupError represents a detailed cleanup error with context
type CleanupError struct {
	Type        CleanupErrorType
	SnapshotID  string
	SessionID   string
	TaskID      string
	Retryable   bool
	RetryAfter  time.Duration
	Attempt     int
	Underlying  error
	Timestamp   time.Time
	Context     map[string]interface{}
}

// Error implements the error interface
func (e *CleanupError) Error() string {
	return fmt.Sprintf("cleanup error for snapshot %s (session: %s, task: %s, attempt: %d, type: %v): %v",
		e.SnapshotID, e.SessionID, e.TaskID, e.Attempt, e.Type, e.Underlying)
}

// IsRetryable returns true if the error is retryable
func (e *CleanupError) IsRetryable() bool {
	return e.Retryable && e.Type != CleanupErrorPermanent && e.Type != CleanupErrorAuth
}

// ClassifyError analyzes an error and returns a classified CleanupError
func ClassifyError(err error, snapshotID, sessionID, taskID string, attempt int) *CleanupError {
	if err == nil {
		return nil
	}

	cleanupErr := &CleanupError{
		SnapshotID: snapshotID,
		SessionID:  sessionID,
		TaskID:     taskID,
		Attempt:    attempt,
		Underlying: err,
		Timestamp:  time.Now(),
		Context:    make(map[string]interface{}),
	}

	errStr := err.Error()
	
	// Classify based on error message patterns
	switch {
	case containsAny(errStr, []string{"not found", "404", "does not exist"}):
		cleanupErr.Type = CleanupErrorNotFound
		cleanupErr.Retryable = false
	case containsAny(errStr, []string{"unauthorized", "forbidden", "403", "401", "permission"}):
		cleanupErr.Type = CleanupErrorAuth
		cleanupErr.Retryable = false
	case containsAny(errStr, []string{"rate limit", "quota", "too many requests", "429"}):
		cleanupErr.Type = CleanupErrorRateLimit
		cleanupErr.Retryable = true
		cleanupErr.RetryAfter = calculateBackoff(attempt, 30*time.Second)
	case containsAny(errStr, []string{"timeout", "deadline", "context canceled"}):
		cleanupErr.Type = CleanupErrorTimeout
		cleanupErr.Retryable = true
		cleanupErr.RetryAfter = calculateBackoff(attempt, 10*time.Second)
	case containsAny(errStr, []string{"temporary", "transient", "retry", "503", "502", "500"}):
		cleanupErr.Type = CleanupErrorTransient
		cleanupErr.Retryable = true
		cleanupErr.RetryAfter = calculateBackoff(attempt, 5*time.Second)
	default:
		cleanupErr.Type = CleanupErrorPermanent
		cleanupErr.Retryable = false
	}

	return cleanupErr
}

// containsAny checks if a string contains any of the given substrings
func containsAny(str string, substrings []string) bool {
	for _, substr := range substrings {
		if len(str) >= len(substr) {
			for i := 0; i <= len(str)-len(substr); i++ {
				if str[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int, baseDelay time.Duration) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}
	
	// Exponential backoff: base * 2^attempt
	delay := baseDelay
	for i := 0; i < attempt && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	
	// Cap at 5 minutes
	if delay > 5*time.Minute {
		delay = 5*time.Minute
	}
	
	// Add 10% jitter
	jitter := time.Duration(float64(delay) * 0.1)
	return delay + jitter
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker implements circuit breaker pattern for cleanup operations
type CircuitBreaker struct {
	name             string
	maxFailures      int64
	resetTimeout     time.Duration
	state            CircuitBreakerState
	failures         int64
	lastFailureTime  time.Time
	halfOpenAttempts int64
	maxHalfOpen      int64
	mu               sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, maxFailures int64, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
		maxHalfOpen:  3, // Allow 3 half-open attempts
	}
}

// Execute runs a function through the circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if we should transition from OPEN to HALF_OPEN
	if cb.state == CircuitOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		cb.state = CircuitHalfOpen
		cb.halfOpenAttempts = 0
		logger.Op.WithFields(map[string]interface{}{
			"circuitBreaker": cb.name,
			"state":          "half-open",
		}).Info("Circuit breaker transitioning to half-open state")
	}

	// If circuit is open, fail fast
	if cb.state == CircuitOpen {
		return fmt.Errorf("circuit breaker %s is open", cb.name)
	}

	// If half-open, limit concurrent attempts
	if cb.state == CircuitHalfOpen && cb.halfOpenAttempts >= cb.maxHalfOpen {
		return fmt.Errorf("circuit breaker %s is half-open with max attempts reached", cb.name)
	}

	if cb.state == CircuitHalfOpen {
		cb.halfOpenAttempts++
	}

	// Execute the function
	err := fn()
	
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// recordFailure records a failure and potentially opens the circuit
func (cb *CircuitBreaker) recordFailure() {
	atomic.AddInt64(&cb.failures, 1)
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
		logger.Op.WithFields(map[string]interface{}{
			"circuitBreaker": cb.name,
			"failures":       cb.failures,
			"state":          "open",
		}).Warn("Circuit breaker opened due to consecutive failures")
	}
}

// recordSuccess records a success and potentially closes the circuit
func (cb *CircuitBreaker) recordSuccess() {
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		logger.Op.WithFields(map[string]interface{}{
			"circuitBreaker": cb.name,
			"state":          "closed",
		}).Info("Circuit breaker closed after successful operation")
	}
	atomic.StoreInt64(&cb.failures, 0)
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// HealthChecker monitors cleanup operation health
type HealthChecker struct {
	checkInterval   time.Duration
	successWindow   time.Duration
	failureWindow   time.Duration
	minSuccessRate  float64
	recentResults   []HealthCheckResult
	mu              sync.RWMutex
	isHealthy       bool
	lastCheck       time.Time
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Timestamp time.Time
	Success   bool
	Duration  time.Duration
	Error     error
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(checkInterval, successWindow, failureWindow time.Duration, minSuccessRate float64) *HealthChecker {
	return &HealthChecker{
		checkInterval:  checkInterval,
		successWindow:  successWindow,
		failureWindow:  failureWindow,
		minSuccessRate: minSuccessRate,
		recentResults:  make([]HealthCheckResult, 0),
		isHealthy:      true,
	}
}

// RecordResult records a health check result
func (hc *HealthChecker) RecordResult(success bool, duration time.Duration, err error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	result := HealthCheckResult{
		Timestamp: time.Now(),
		Success:   success,
		Duration:  duration,
		Error:     err,
	}

	hc.recentResults = append(hc.recentResults, result)
	hc.cleanupOldResults()
	hc.updateHealthStatus()
}

// cleanupOldResults removes results outside the monitoring window
func (hc *HealthChecker) cleanupOldResults() {
	cutoff := time.Now().Add(-hc.successWindow)
	filtered := make([]HealthCheckResult, 0)
	
	for _, result := range hc.recentResults {
		if result.Timestamp.After(cutoff) {
			filtered = append(filtered, result)
		}
	}
	
	hc.recentResults = filtered
}

// updateHealthStatus calculates current health status
func (hc *HealthChecker) updateHealthStatus() {
	if len(hc.recentResults) == 0 {
		hc.isHealthy = true
		return
	}

	successCount := 0
	for _, result := range hc.recentResults {
		if result.Success {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(len(hc.recentResults))
	hc.isHealthy = successRate >= hc.minSuccessRate
	hc.lastCheck = time.Now()
}

// IsHealthy returns the current health status
func (hc *HealthChecker) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.isHealthy
}

// GetStats returns health statistics
func (hc *HealthChecker) GetStats() (total, successful int, successRate float64, lastCheck time.Time) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	total = len(hc.recentResults)
	successful = 0
	for _, result := range hc.recentResults {
		if result.Success {
			successful++
		}
	}

	if total > 0 {
		successRate = float64(successful) / float64(total)
	}

	return total, successful, successRate, hc.lastCheck
}

// ResilientCleanupManager wraps MultiLevelCleanupManager with resilience features
type ResilientCleanupManager struct {
	*MultiLevelCleanupManager
	circuitBreaker  *CircuitBreaker
	healthChecker   *HealthChecker
	errorHistory    []CleanupError
	maxErrorHistory int
	mu              sync.RWMutex
}

// NewResilientCleanupManager creates a new resilient cleanup manager
func NewResilientCleanupManager(config *Config, gcpClient *gcp.Clients, strategy *CleanupStrategy, sessionID string) *ResilientCleanupManager {
	baseManager := NewMultiLevelCleanupManager(config, gcpClient, strategy, sessionID)
	
	return &ResilientCleanupManager{
		MultiLevelCleanupManager: baseManager,
		circuitBreaker:          NewCircuitBreaker("cleanup-operations", 5, 2*time.Minute),
		healthChecker:           NewHealthChecker(30*time.Second, 5*time.Minute, 1*time.Minute, 0.8),
		errorHistory:            make([]CleanupError, 0),
		maxErrorHistory:         100,
	}
}

// ResilientCleanupTaskSnapshot performs resilient task-level cleanup
func (rcm *ResilientCleanupManager) ResilientCleanupTaskSnapshot(ctx context.Context, taskID, snapshotName string) *CleanupResult {
	startTime := time.Now()
	
	var result *CleanupResult
	err := rcm.circuitBreaker.Execute(func() error {
		result = rcm.CleanupTaskSnapshot(ctx, taskID, snapshotName)
		if len(result.Errors) > 0 {
			return result.Errors[0]
		}
		return nil
	})

	duration := time.Since(startTime)
	success := err == nil && len(result.Errors) == 0

	// Record health check result
	rcm.healthChecker.RecordResult(success, duration, err)
	
	// Record retry attempts
	if !success {
		rcm.MultiLevelCleanupManager.monitor.metrics.RecordRetryAttempt(false)
	} else {
		rcm.MultiLevelCleanupManager.monitor.metrics.RecordRetryAttempt(true)
	}

	// Record errors in history
	if !success {
		for _, cleanupErr := range result.Errors {
			classifiedErr := ClassifyError(cleanupErr, snapshotName, rcm.sessionID, taskID, 1)
			rcm.recordError(*classifiedErr)
		}
	}

	// If circuit breaker failed, create appropriate result
	if err != nil && result == nil {
		result = &CleanupResult{
			Level:           CleanupLevelTask,
			SessionID:       rcm.sessionID,
			TaskID:          taskID,
			SnapshotsFound:  1,
			SnapshotsDeleted: 0,
			SnapshotsFailed: []string{snapshotName},
			Errors:          []error{err},
			Duration:        duration,
		}
	}

	return result
}

// ResilientCleanupSessionSnapshots performs resilient session-level cleanup
func (rcm *ResilientCleanupManager) ResilientCleanupSessionSnapshots(ctx context.Context) *CleanupResult {
	startTime := time.Now()
	
	var result *CleanupResult
	err := rcm.circuitBreaker.Execute(func() error {
		result = rcm.CleanupSessionSnapshots(ctx)
		if len(result.Errors) > 0 {
			// Only fail circuit if more than 50% of snapshots failed
			if len(result.SnapshotsFailed) > result.SnapshotsFound/2 {
				return fmt.Errorf("majority of snapshots failed cleanup: %d/%d", len(result.SnapshotsFailed), result.SnapshotsFound)
			}
		}
		return nil
	})

	duration := time.Since(startTime)
	success := err == nil && len(result.SnapshotsFailed) < result.SnapshotsFound/2

	// Record health check result
	rcm.healthChecker.RecordResult(success, duration, err)

	// Record errors in history
	if len(result.Errors) > 0 {
		for i, cleanupErr := range result.Errors {
			snapshotName := ""
			if i < len(result.SnapshotsFailed) {
				snapshotName = result.SnapshotsFailed[i]
			}
			classifiedErr := ClassifyError(cleanupErr, snapshotName, rcm.sessionID, "", 1)
			rcm.recordError(*classifiedErr)
		}
	}

	// If circuit breaker failed, create appropriate result
	if err != nil && result == nil {
		result = &CleanupResult{
			Level:     CleanupLevelSession,
			SessionID: rcm.sessionID,
			Errors:    []error{err},
			Duration:  duration,
		}
	}

	return result
}

// recordError adds an error to the error history
func (rcm *ResilientCleanupManager) recordError(err CleanupError) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	rcm.errorHistory = append(rcm.errorHistory, err)
	
	// Trim history if it exceeds max size
	if len(rcm.errorHistory) > rcm.maxErrorHistory {
		rcm.errorHistory = rcm.errorHistory[len(rcm.errorHistory)-rcm.maxErrorHistory:]
	}
}

// GetErrorHistory returns recent cleanup errors
func (rcm *ResilientCleanupManager) GetErrorHistory() []CleanupError {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	history := make([]CleanupError, len(rcm.errorHistory))
	copy(history, rcm.errorHistory)
	return history
}

// GetHealthStatus returns current health status and statistics
func (rcm *ResilientCleanupManager) GetHealthStatus() (healthy bool, circuitState CircuitBreakerState, stats map[string]interface{}) {
	healthy = rcm.healthChecker.IsHealthy()
	circuitState = rcm.circuitBreaker.GetState()
	
	total, successful, successRate, lastCheck := rcm.healthChecker.GetStats()
	
	stats = map[string]interface{}{
		"healthy":         healthy,
		"circuit_state":   circuitState,
		"total_checks":    total,
		"successful":      successful,
		"success_rate":    successRate,
		"last_check":      lastCheck,
		"error_count":     len(rcm.GetErrorHistory()),
		"active_snapshots": rcm.GetActiveSnapshotCount(),
	}
	
	return healthy, circuitState, stats
}