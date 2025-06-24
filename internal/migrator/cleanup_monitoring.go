package migrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
)

// AlertLevel represents the severity of an alert
type AlertLevel int

const (
	AlertInfo AlertLevel = iota
	AlertWarning
	AlertError
	AlertCritical
)

// String returns string representation of AlertLevel
func (al AlertLevel) String() string {
	switch al {
	case AlertInfo:
		return "INFO"
	case AlertWarning:
		return "WARNING"
	case AlertError:
		return "ERROR"
	case AlertCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Alert represents a monitoring alert
type Alert struct {
	ID          string                 `json:"id"`
	Level       AlertLevel             `json:"level"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	SessionID   string                 `json:"session_id"`
	Component   string                 `json:"component"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CleanupMetrics holds cleanup operation metrics
type CleanupMetrics struct {
	SessionID               string                 `json:"session_id"`
	StartTime               time.Time              `json:"start_time"`
	LastUpdateTime          time.Time              `json:"last_update_time"`
	TotalSnapshots          int64                  `json:"total_snapshots"`
	SnapshotsDeleted        int64                  `json:"snapshots_deleted"`
	SnapshotsFailed         int64                  `json:"snapshots_failed"`
	TaskLevelCleanups       int64                  `json:"task_level_cleanups"`
	SessionLevelCleanups    int64                  `json:"session_level_cleanups"`
	EmergencyCleanups       int64                  `json:"emergency_cleanups"`
	CircuitBreakerTrips     int64                  `json:"circuit_breaker_trips"`
	RetryAttempts           int64                  `json:"retry_attempts"`
	SuccessfulRetries       int64                  `json:"successful_retries"`
	HealthCheckFailures     int64                  `json:"health_check_failures"`
	AverageCleanupDuration  time.Duration          `json:"average_cleanup_duration"`
	ErrorsByType            map[string]int64       `json:"errors_by_type"`
	LastCleanupResult       *CleanupResult         `json:"last_cleanup_result,omitempty"`
	Alerts                  []Alert                `json:"alerts"`
	CustomMetrics           map[string]interface{} `json:"custom_metrics"`
	mu                      sync.RWMutex
}

// NewCleanupMetrics creates a new metrics collector
func NewCleanupMetrics(sessionID string) *CleanupMetrics {
	return &CleanupMetrics{
		SessionID:      sessionID,
		StartTime:      time.Now(),
		LastUpdateTime: time.Now(),
		ErrorsByType:   make(map[string]int64),
		Alerts:         make([]Alert, 0),
		CustomMetrics:  make(map[string]interface{}),
	}
}

// RecordCleanupResult records the result of a cleanup operation
func (cm *CleanupMetrics) RecordCleanupResult(result *CleanupResult) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.LastUpdateTime = time.Now()
	cm.LastCleanupResult = result
	cm.TotalSnapshots += int64(result.SnapshotsFound)
	cm.SnapshotsDeleted += int64(result.SnapshotsDeleted)
	cm.SnapshotsFailed += int64(len(result.SnapshotsFailed))

	// Count cleanup operations by level
	switch result.Level {
	case CleanupLevelTask:
		cm.TaskLevelCleanups++
	case CleanupLevelSession:
		cm.SessionLevelCleanups++
	case CleanupLevelEmergency:
		cm.EmergencyCleanups++
	}

	// Record error types
	for _, err := range result.Errors {
		classifiedErr := ClassifyError(err, "", cm.SessionID, "", 1)
		errorType := classifiedErr.Type.String()
		cm.ErrorsByType[errorType]++
	}

	// Update average cleanup duration
	if result.Duration > 0 {
		totalOps := cm.TaskLevelCleanups + cm.SessionLevelCleanups + cm.EmergencyCleanups
		if totalOps > 0 {
			cm.AverageCleanupDuration = (cm.AverageCleanupDuration*time.Duration(totalOps-1) + result.Duration) / time.Duration(totalOps)
		}
	}

	// Generate alerts based on results
	cm.generateAlertsFromResult(result)
}

// String method for CleanupErrorType
func (cet CleanupErrorType) String() string {
	switch cet {
	case CleanupErrorTransient:
		return "transient"
	case CleanupErrorPermanent:
		return "permanent"
	case CleanupErrorRateLimit:
		return "rate_limit"
	case CleanupErrorTimeout:
		return "timeout"
	case CleanupErrorAuth:
		return "auth"
	case CleanupErrorNotFound:
		return "not_found"
	default:
		return "unknown"
	}
}

// generateAlertsFromResult generates alerts based on cleanup results
func (cm *CleanupMetrics) generateAlertsFromResult(result *CleanupResult) {
	now := time.Now()

	// Alert for failed cleanups
	if len(result.SnapshotsFailed) > 0 {
		failureRate := float64(len(result.SnapshotsFailed)) / float64(result.SnapshotsFound)
		
		var level AlertLevel
		switch {
		case failureRate >= 0.8:
			level = AlertCritical
		case failureRate >= 0.5:
			level = AlertError
		case failureRate >= 0.2:
			level = AlertWarning
		default:
			level = AlertInfo
		}

		alert := Alert{
			ID:          fmt.Sprintf("cleanup-failure-%s-%d", cm.SessionID, now.Unix()),
			Level:       level,
			Title:       "Cleanup Operation Failures",
			Description: fmt.Sprintf("%.1f%% of snapshots failed cleanup (%d/%d)", failureRate*100, len(result.SnapshotsFailed), result.SnapshotsFound),
			SessionID:   cm.SessionID,
			Component:   "cleanup",
			Timestamp:   now,
			Metadata: map[string]interface{}{
				"failure_rate":      failureRate,
				"failed_snapshots":  result.SnapshotsFailed,
				"cleanup_level":     result.Level.String(),
				"cleanup_duration":  result.Duration,
			},
		}
		cm.Alerts = append(cm.Alerts, alert)
	}

	// Alert for slow cleanup operations
	if result.Duration > 5*time.Minute {
		alert := Alert{
			ID:          fmt.Sprintf("cleanup-slow-%s-%d", cm.SessionID, now.Unix()),
			Level:       AlertWarning,
			Title:       "Slow Cleanup Operation",
			Description: fmt.Sprintf("Cleanup operation took %v, which exceeds normal duration", result.Duration),
			SessionID:   cm.SessionID,
			Component:   "cleanup",
			Timestamp:   now,
			Metadata: map[string]interface{}{
				"duration":      result.Duration,
				"cleanup_level": result.Level.String(),
				"snapshots":     result.SnapshotsFound,
			},
		}
		cm.Alerts = append(cm.Alerts, alert)
	}
}

// RecordCircuitBreakerTrip records a circuit breaker trip
func (cm *CleanupMetrics) RecordCircuitBreakerTrip() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.CircuitBreakerTrips++
	cm.LastUpdateTime = time.Now()

	alert := Alert{
		ID:          fmt.Sprintf("circuit-breaker-%s-%d", cm.SessionID, time.Now().Unix()),
		Level:       AlertError,
		Title:       "Circuit Breaker Tripped",
		Description: "Cleanup circuit breaker has opened due to consecutive failures",
		SessionID:   cm.SessionID,
		Component:   "circuit-breaker",
		Timestamp:   time.Now(),
		Metadata: map[string]interface{}{
			"total_trips": cm.CircuitBreakerTrips,
		},
	}
	cm.Alerts = append(cm.Alerts, alert)
}

// RecordRetryAttempt records a retry attempt
func (cm *CleanupMetrics) RecordRetryAttempt(successful bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.RetryAttempts++
	if successful {
		cm.SuccessfulRetries++
	}
	cm.LastUpdateTime = time.Now()
}

// RecordHealthCheckFailure records a health check failure
func (cm *CleanupMetrics) RecordHealthCheckFailure() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.HealthCheckFailures++
	cm.LastUpdateTime = time.Now()

	// Generate alert for repeated health check failures
	if cm.HealthCheckFailures%5 == 0 { // Every 5 failures
		alert := Alert{
			ID:          fmt.Sprintf("health-check-%s-%d", cm.SessionID, time.Now().Unix()),
			Level:       AlertWarning,
			Title:       "Repeated Health Check Failures",
			Description: fmt.Sprintf("Health checks have failed %d times", cm.HealthCheckFailures),
			SessionID:   cm.SessionID,
			Component:   "health-check",
			Timestamp:   time.Now(),
			Metadata: map[string]interface{}{
				"failure_count": cm.HealthCheckFailures,
			},
		}
		cm.Alerts = append(cm.Alerts, alert)
	}
}

// SetCustomMetric sets a custom metric value
func (cm *CleanupMetrics) SetCustomMetric(key string, value interface{}) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.CustomMetrics[key] = value
	cm.LastUpdateTime = time.Now()
}

// GetMetrics returns a copy of current metrics
func (cm *CleanupMetrics) GetMetrics() CleanupMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Create a deep copy
	metrics := CleanupMetrics{
		SessionID:               cm.SessionID,
		StartTime:               cm.StartTime,
		LastUpdateTime:          cm.LastUpdateTime,
		TotalSnapshots:          cm.TotalSnapshots,
		SnapshotsDeleted:        cm.SnapshotsDeleted,
		SnapshotsFailed:         cm.SnapshotsFailed,
		TaskLevelCleanups:       cm.TaskLevelCleanups,
		SessionLevelCleanups:    cm.SessionLevelCleanups,
		EmergencyCleanups:       cm.EmergencyCleanups,
		CircuitBreakerTrips:     cm.CircuitBreakerTrips,
		RetryAttempts:           cm.RetryAttempts,
		SuccessfulRetries:       cm.SuccessfulRetries,
		HealthCheckFailures:     cm.HealthCheckFailures,
		AverageCleanupDuration:  cm.AverageCleanupDuration,
		ErrorsByType:            make(map[string]int64),
		CustomMetrics:           make(map[string]interface{}),
		Alerts:                  make([]Alert, len(cm.Alerts)),
	}

	// Copy maps and slices
	for k, v := range cm.ErrorsByType {
		metrics.ErrorsByType[k] = v
	}
	for k, v := range cm.CustomMetrics {
		metrics.CustomMetrics[k] = v
	}
	copy(metrics.Alerts, cm.Alerts)

	return metrics
}

// GetActiveAlerts returns unresolved alerts
func (cm *CleanupMetrics) GetActiveAlerts() []Alert {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	activeAlerts := make([]Alert, 0)
	for _, alert := range cm.Alerts {
		if !alert.Resolved {
			activeAlerts = append(activeAlerts, alert)
		}
	}
	return activeAlerts
}

// ResolveAlert marks an alert as resolved
func (cm *CleanupMetrics) ResolveAlert(alertID string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i := range cm.Alerts {
		if cm.Alerts[i].ID == alertID && !cm.Alerts[i].Resolved {
			now := time.Now()
			cm.Alerts[i].Resolved = true
			cm.Alerts[i].ResolvedAt = &now
			return true
		}
	}
	return false
}

// CleanupMonitor provides comprehensive monitoring for cleanup operations
type CleanupMonitor struct {
	metrics          *CleanupMetrics
	alertHandlers    []AlertHandler
	healthThresholds HealthThresholds
	mu               sync.RWMutex
}

// AlertHandler defines the interface for handling alerts
type AlertHandler interface {
	HandleAlert(alert Alert) error
}

// HealthThresholds defines thresholds for health monitoring
type HealthThresholds struct {
	MaxFailureRate     float64       // Maximum acceptable failure rate (0.0-1.0)
	MaxCleanupDuration time.Duration // Maximum acceptable cleanup duration
	MaxRetryAttempts   int64         // Maximum retry attempts before alerting
	HealthCheckWindow  time.Duration // Time window for health check evaluation
}

// DefaultHealthThresholds returns sensible default health thresholds
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		MaxFailureRate:     0.1,  // 10% failure rate
		MaxCleanupDuration: 10 * time.Minute,
		MaxRetryAttempts:   10,
		HealthCheckWindow:  5 * time.Minute,
	}
}

// LogAlertHandler logs alerts using the internal logger
type LogAlertHandler struct{}

// HandleAlert logs the alert
func (lah *LogAlertHandler) HandleAlert(alert Alert) error {
	logFields := map[string]interface{}{
		"alert_id":    alert.ID,
		"level":       alert.Level.String(),
		"component":   alert.Component,
		"session_id":  alert.SessionID,
		"timestamp":   alert.Timestamp,
	}

	for k, v := range alert.Metadata {
		logFields[fmt.Sprintf("meta_%s", k)] = v
	}

	switch alert.Level {
	case AlertInfo:
		logger.Op.WithFields(logFields).Infof("ALERT: %s - %s", alert.Title, alert.Description)
	case AlertWarning:
		logger.Op.WithFields(logFields).Warnf("ALERT: %s - %s", alert.Title, alert.Description)
	case AlertError, AlertCritical:
		logger.Op.WithFields(logFields).Errorf("ALERT: %s - %s", alert.Title, alert.Description)
	}

	return nil
}

// NewCleanupMonitor creates a new cleanup monitor
func NewCleanupMonitor(sessionID string, thresholds HealthThresholds) *CleanupMonitor {
	monitor := &CleanupMonitor{
		metrics:          NewCleanupMetrics(sessionID),
		alertHandlers:    make([]AlertHandler, 0),
		healthThresholds: thresholds,
	}

	// Add default log alert handler
	monitor.AddAlertHandler(&LogAlertHandler{})

	return monitor
}

// AddAlertHandler adds an alert handler
func (cm *CleanupMonitor) AddAlertHandler(handler AlertHandler) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.alertHandlers = append(cm.alertHandlers, handler)
}

// RecordCleanupResult records a cleanup result and triggers monitoring
func (cm *CleanupMonitor) RecordCleanupResult(result *CleanupResult) {
	cm.metrics.RecordCleanupResult(result)
	cm.checkHealthAndAlert()
}

// RecordCircuitBreakerTrip records a circuit breaker trip
func (cm *CleanupMonitor) RecordCircuitBreakerTrip() {
	cm.metrics.RecordCircuitBreakerTrip()
	cm.triggerAlerts(cm.metrics.GetActiveAlerts())
}

// checkHealthAndAlert evaluates health and triggers alerts if needed
func (cm *CleanupMonitor) checkHealthAndAlert() {
	metrics := cm.metrics.GetMetrics()
	
	// Check failure rate
	if metrics.TotalSnapshots > 0 {
		failureRate := float64(metrics.SnapshotsFailed) / float64(metrics.TotalSnapshots)
		if failureRate > cm.healthThresholds.MaxFailureRate {
			alert := Alert{
				ID:          fmt.Sprintf("high-failure-rate-%s-%d", metrics.SessionID, time.Now().Unix()),
				Level:       AlertError,
				Title:       "High Cleanup Failure Rate",
				Description: fmt.Sprintf("Cleanup failure rate (%.1f%%) exceeds threshold (%.1f%%)", failureRate*100, cm.healthThresholds.MaxFailureRate*100),
				SessionID:   metrics.SessionID,
				Component:   "monitoring",
				Timestamp:   time.Now(),
				Metadata: map[string]interface{}{
					"failure_rate": failureRate,
					"threshold":    cm.healthThresholds.MaxFailureRate,
					"total":        metrics.TotalSnapshots,
					"failed":       metrics.SnapshotsFailed,
				},
			}
			cm.metrics.Alerts = append(cm.metrics.Alerts, alert)
		}
	}

	// Check retry attempts
	if metrics.RetryAttempts > cm.healthThresholds.MaxRetryAttempts {
		alert := Alert{
			ID:          fmt.Sprintf("high-retry-count-%s-%d", metrics.SessionID, time.Now().Unix()),
			Level:       AlertWarning,
			Title:       "High Retry Attempt Count",
			Description: fmt.Sprintf("Retry attempts (%d) exceed threshold (%d)", metrics.RetryAttempts, cm.healthThresholds.MaxRetryAttempts),
			SessionID:   metrics.SessionID,
			Component:   "monitoring",
			Timestamp:   time.Now(),
			Metadata: map[string]interface{}{
				"retry_attempts": metrics.RetryAttempts,
				"threshold":      cm.healthThresholds.MaxRetryAttempts,
				"successful":     metrics.SuccessfulRetries,
			},
		}
		cm.metrics.Alerts = append(cm.metrics.Alerts, alert)
	}

	// Trigger alerts
	cm.triggerAlerts(cm.metrics.GetActiveAlerts())
}

// triggerAlerts sends alerts to all handlers
func (cm *CleanupMonitor) triggerAlerts(alerts []Alert) {
	cm.mu.RLock()
	handlers := make([]AlertHandler, len(cm.alertHandlers))
	copy(handlers, cm.alertHandlers)
	cm.mu.RUnlock()

	for _, alert := range alerts {
		for _, handler := range handlers {
			if err := handler.HandleAlert(alert); err != nil {
				logger.Op.WithFields(map[string]interface{}{
					"alert_id": alert.ID,
					"error":    err.Error(),
				}).Error("Failed to handle alert")
			}
		}
	}
}

// GetMetrics returns current metrics
func (cm *CleanupMonitor) GetMetrics() CleanupMetrics {
	return cm.metrics.GetMetrics()
}

// GetHealthStatus returns current health status
func (cm *CleanupMonitor) GetHealthStatus() map[string]interface{} {
	metrics := cm.metrics.GetMetrics()
	activeAlerts := cm.metrics.GetActiveAlerts()

	status := map[string]interface{}{
		"session_id":        metrics.SessionID,
		"uptime":            time.Since(metrics.StartTime),
		"last_update":       metrics.LastUpdateTime,
		"total_snapshots":   metrics.TotalSnapshots,
		"snapshots_deleted": metrics.SnapshotsDeleted,
		"snapshots_failed":  metrics.SnapshotsFailed,
		"success_rate":      0.0,
		"active_alerts":     len(activeAlerts),
		"circuit_trips":     metrics.CircuitBreakerTrips,
		"retry_attempts":    metrics.RetryAttempts,
		"health_failures":   metrics.HealthCheckFailures,
		"avg_duration":      metrics.AverageCleanupDuration,
		"errors_by_type":    metrics.ErrorsByType,
	}

	if metrics.TotalSnapshots > 0 {
		status["success_rate"] = float64(metrics.SnapshotsDeleted) / float64(metrics.TotalSnapshots)
	}

	return status
}

// StartPeriodicHealthCheck starts a periodic health check routine
func (cm *CleanupMonitor) StartPeriodicHealthCheck(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.checkHealthAndAlert()
		}
	}
}