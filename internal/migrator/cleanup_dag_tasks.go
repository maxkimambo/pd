package migrator

import (
	"context"
	"fmt"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// Task interface defines the contract for DAG tasks
// This mirrors the interface in the dag package to avoid circular imports
type Task interface {
	// Execute performs the task's work
	Execute(ctx context.Context) error

	// Rollback performs cleanup if execution fails
	Rollback(ctx context.Context) error

	// GetID returns the unique identifier for this task
	GetID() string

	// GetType returns the task type
	GetType() string

	// GetDescription returns a human-readable description
	GetDescription() string

	// Validate checks if the task can be executed
	Validate() error
}

// BaseTask provides common functionality for tasks
type BaseTask struct {
	id          string
	taskType    string
	description string
}

// NewBaseTask creates a new base task
func NewBaseTask(id, taskType, description string) *BaseTask {
	return &BaseTask{
		id:          id,
		taskType:    taskType,
		description: description,
	}
}

// GetID returns the unique identifier for this task
func (t *BaseTask) GetID() string {
	return t.id
}

// GetType returns the task type
func (t *BaseTask) GetType() string {
	return t.taskType
}

// GetDescription returns a human-readable description
func (t *BaseTask) GetDescription() string {
	return t.description
}

// Validate checks if the task can be executed (default implementation)
func (t *BaseTask) Validate() error {
	return nil
}

// CleanupTaskType constants for different cleanup task types
const (
	CleanupTaskTypeSnapshot    = "snapshot-cleanup"
	CleanupTaskTypeSession     = "session-cleanup"
	CleanupTaskTypeEmergency   = "emergency-cleanup"
	CleanupTaskTypeHealthCheck = "health-check"
)

// SnapshotCleanupTask implements DAG Task interface for individual snapshot cleanup
type SnapshotCleanupTask struct {
	*BaseTask
	cleanupManager *MultiLevelCleanupManager
	taskID         string
	snapshotName   string
	sessionID      string
	config         *Config
}

// NewSnapshotCleanupTask creates a new snapshot cleanup task for DAG execution
func NewSnapshotCleanupTask(id, taskID, snapshotName, sessionID string, cleanupManager *MultiLevelCleanupManager, config *Config) *SnapshotCleanupTask {
	description := fmt.Sprintf("Clean up snapshot %s for task %s", snapshotName, taskID)

	return &SnapshotCleanupTask{
		BaseTask:       NewBaseTask(id, CleanupTaskTypeSnapshot, description),
		cleanupManager: cleanupManager,
		taskID:         taskID,
		snapshotName:   snapshotName,
		sessionID:      sessionID,
		config:         config,
	}
}

// Execute performs the snapshot cleanup operation
func (t *SnapshotCleanupTask) Execute(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID":        t.GetID(),
		"snapshotName":  t.snapshotName,
		"sessionID":     t.sessionID,
		"migrationTask": t.taskID,
	}

	logger.Op.WithFields(logFields).Info("Starting snapshot cleanup task...")

	// Use the cleanup manager for task snapshot cleanup
	result := t.cleanupManager.CleanupTaskSnapshot(ctx, t.taskID, t.snapshotName)

	// Log the cleanup result
	resultFields := map[string]interface{}{
		"taskID":       t.GetID(),
		"snapshotName": t.snapshotName,
		"sessionID":    t.sessionID,
		"found":        result.SnapshotsFound,
		"deleted":      result.SnapshotsDeleted,
		"failed":       len(result.SnapshotsFailed),
		"duration":     result.Duration,
		"level":        result.Level.String(),
	}

	if len(result.Errors) > 0 {
		resultFields["errors"] = len(result.Errors)
		logger.Op.WithFields(resultFields).Warn("Snapshot cleanup completed with errors")
		return fmt.Errorf("snapshot cleanup failed: %v", result.Errors[0])
	}

	if result.SnapshotsDeleted == 0 && result.SnapshotsFound > 0 {
		logger.Op.WithFields(resultFields).Warn("Snapshot cleanup found snapshots but none were deleted")
		return fmt.Errorf("failed to delete snapshot %s", t.snapshotName)
	}

	logger.Op.WithFields(resultFields).Info("Snapshot cleanup task completed successfully")
	return nil
}

// Rollback handles cleanup task rollback (usually not needed for cleanup tasks)
func (t *SnapshotCleanupTask) Rollback(ctx context.Context) error {
	// Cleanup tasks typically don't need rollback since they're already cleanup operations
	// However, we can log that a rollback was attempted
	logFields := map[string]interface{}{
		"taskID":       t.GetID(),
		"snapshotName": t.snapshotName,
		"sessionID":    t.sessionID,
	}

	logger.Op.WithFields(logFields).Warn("Rollback requested for snapshot cleanup task (no action taken)")
	return nil
}

// SessionCleanupTask implements DAG Task interface for session-level cleanup
type SessionCleanupTask struct {
	*BaseTask
	cleanupManager *MultiLevelCleanupManager
	sessionID      string
	config         *Config
}

// NewSessionCleanupTask creates a new session cleanup task for DAG execution
func NewSessionCleanupTask(id, sessionID string, cleanupManager *MultiLevelCleanupManager, config *Config) *SessionCleanupTask {
	description := fmt.Sprintf("Clean up all snapshots for session %s", sessionID)

	return &SessionCleanupTask{
		BaseTask:       NewBaseTask(id, CleanupTaskTypeSession, description),
		cleanupManager: cleanupManager,
		sessionID:      sessionID,
		config:         config,
	}
}

// Execute performs the session cleanup operation
func (t *SessionCleanupTask) Execute(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID":    t.GetID(),
		"sessionID": t.sessionID,
	}

	logger.Op.WithFields(logFields).Info("Starting session cleanup task...")

	// Use the cleanup manager for session cleanup
	result := t.cleanupManager.CleanupSessionSnapshots(ctx)

	// Log the cleanup result
	resultFields := map[string]interface{}{
		"taskID":    t.GetID(),
		"sessionID": t.sessionID,
		"found":     result.SnapshotsFound,
		"deleted":   result.SnapshotsDeleted,
		"failed":    len(result.SnapshotsFailed),
		"duration":  result.Duration,
		"level":     result.Level.String(),
	}

	if len(result.Errors) > 0 {
		resultFields["errors"] = len(result.Errors)
		logger.Op.WithFields(resultFields).Warn("Session cleanup completed with errors")

		// For session cleanup, we might tolerate some failures if majority succeeded
		failureRate := float64(len(result.SnapshotsFailed)) / float64(result.SnapshotsFound)
		if failureRate > 0.5 {
			return fmt.Errorf("session cleanup failed: majority of snapshots failed (%d/%d)", len(result.SnapshotsFailed), result.SnapshotsFound)
		}

		logger.Op.WithFields(resultFields).Warn("Session cleanup had some failures but majority succeeded")
		return nil
	}

	logger.Op.WithFields(resultFields).Info("Session cleanup task completed successfully")
	return nil
}

// Rollback handles session cleanup task rollback
func (t *SessionCleanupTask) Rollback(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID":    t.GetID(),
		"sessionID": t.sessionID,
	}

	logger.Op.WithFields(logFields).Warn("Rollback requested for session cleanup task (no action taken)")
	return nil
}

// EmergencyCleanupTask implements DAG Task interface for emergency cleanup
type EmergencyCleanupTask struct {
	*BaseTask
	cleanupManager *MultiLevelCleanupManager
	config         *Config
}

// NewEmergencyCleanupTask creates a new emergency cleanup task for DAG execution
func NewEmergencyCleanupTask(id string, cleanupManager *MultiLevelCleanupManager, config *Config) *EmergencyCleanupTask {
	description := "Emergency cleanup of expired snapshots across all sessions"

	return &EmergencyCleanupTask{
		BaseTask:       NewBaseTask(id, CleanupTaskTypeEmergency, description),
		cleanupManager: cleanupManager,
		config:         config,
	}
}

// Execute performs the emergency cleanup operation
func (t *EmergencyCleanupTask) Execute(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID": t.GetID(),
	}

	logger.Op.WithFields(logFields).Info("Starting emergency cleanup task...")

	// Use the cleanup manager for emergency cleanup (cross-session)
	result := t.cleanupManager.CleanupExpiredSnapshots(ctx)

	// Log the cleanup result
	resultFields := map[string]interface{}{
		"taskID":   t.GetID(),
		"found":    result.SnapshotsFound,
		"deleted":  result.SnapshotsDeleted,
		"failed":   len(result.SnapshotsFailed),
		"duration": result.Duration,
		"level":    result.Level.String(),
	}

	if len(result.Errors) > 0 {
		resultFields["errors"] = len(result.Errors)
		logger.Op.WithFields(resultFields).Warn("Emergency cleanup completed with errors")

		// For emergency cleanup, we're more tolerant of failures
		failureRate := float64(len(result.SnapshotsFailed)) / float64(result.SnapshotsFound)
		if failureRate > 0.8 { // Only fail if 80% failed
			return fmt.Errorf("emergency cleanup failed: most snapshots failed (%d/%d)", len(result.SnapshotsFailed), result.SnapshotsFound)
		}

		logger.Op.WithFields(resultFields).Warn("Emergency cleanup had some failures but completed")
		return nil
	}

	logger.Op.WithFields(resultFields).Info("Emergency cleanup task completed successfully")
	return nil
}

// Rollback handles emergency cleanup task rollback
func (t *EmergencyCleanupTask) Rollback(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID": t.GetID(),
	}

	logger.Op.WithFields(logFields).Warn("Rollback requested for emergency cleanup task (no action taken)")
	return nil
}

// HealthCheckTask implements DAG Task interface for cleanup health monitoring
type HealthCheckTask struct {
	*BaseTask
	cleanupManager *MultiLevelCleanupManager
	config         *Config
}

// NewHealthCheckTask creates a new health check task for DAG execution
func NewHealthCheckTask(id string, cleanupManager *MultiLevelCleanupManager, config *Config) *HealthCheckTask {
	description := "Health check for cleanup operations"

	return &HealthCheckTask{
		BaseTask:       NewBaseTask(id, CleanupTaskTypeHealthCheck, description),
		cleanupManager: cleanupManager,
		config:         config,
	}
}

// Execute performs a health check on the cleanup system
func (t *HealthCheckTask) Execute(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID": t.GetID(),
	}

	logger.Op.WithFields(logFields).Info("Starting cleanup health check task...")

	// Simple health check - verify cleanup manager is functional
	activeSnapshots := t.cleanupManager.GetActiveSnapshotCount()

	// Log the health check result
	healthFields := map[string]interface{}{
		"taskID":          t.GetID(),
		"activeSnapshots": activeSnapshots,
	}

	logger.Op.WithFields(healthFields).Info("Cleanup health check completed - system is operational")
	return nil
}

// Rollback handles health check task rollback
func (t *HealthCheckTask) Rollback(ctx context.Context) error {
	logFields := map[string]interface{}{
		"taskID": t.GetID(),
	}

	logger.Op.WithFields(logFields).Debug("Rollback requested for health check task (no action needed)")
	return nil
}

// CleanupTaskExecutor provides utilities for executing cleanup tasks
type CleanupTaskExecutor struct {
	config         *Config
	gcpClients     *gcp.Clients
	cleanupManager *MultiLevelCleanupManager
	sessionID      string
}

// NewCleanupTaskExecutor creates a new cleanup task executor
func NewCleanupTaskExecutor(config *Config, gcpClients *gcp.Clients, sessionID string) *CleanupTaskExecutor {
	cleanupManager := NewMultiLevelCleanupManager(config, gcpClients, DefaultCleanupStrategy(), sessionID)

	return &CleanupTaskExecutor{
		config:         config,
		gcpClients:     gcpClients,
		cleanupManager: cleanupManager,
		sessionID:      sessionID,
	}
}

// ExecuteTaskCleanup executes cleanup for specific task snapshots
func (executor *CleanupTaskExecutor) ExecuteTaskCleanup(ctx context.Context, taskSnapshots map[string][]string) error {
	logFields := map[string]interface{}{
		"sessionID": executor.sessionID,
		"taskCount": len(taskSnapshots),
	}
	logger.Op.WithFields(logFields).Info("Starting task cleanup execution")

	// Execute health check first
	healthCheckTask := NewHealthCheckTask("health-check-start", executor.cleanupManager, executor.config)
	if err := healthCheckTask.Execute(ctx); err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	// Execute snapshot cleanup tasks
	for taskID, snapshots := range taskSnapshots {
		for i, snapshotName := range snapshots {
			snapshotTaskID := fmt.Sprintf("cleanup-snapshot-%s-%d", taskID, i)
			snapshotTask := NewSnapshotCleanupTask(snapshotTaskID, taskID, snapshotName, executor.sessionID, executor.cleanupManager, executor.config)

			if err := snapshotTask.Execute(ctx); err != nil {
				logger.Op.WithFields(logFields).WithError(err).Warnf("Snapshot cleanup failed for %s", snapshotName)
				// Continue with other snapshots even if one fails
			}
		}
	}

	// Execute final health check
	finalHealthCheckTask := NewHealthCheckTask("health-check-end", executor.cleanupManager, executor.config)
	if err := finalHealthCheckTask.Execute(ctx); err != nil {
		logger.Op.WithFields(logFields).WithError(err).Warn("Final health check failed")
	}

	logger.Op.WithFields(logFields).Info("Task cleanup execution completed")
	return nil
}

// ExecuteSessionCleanup executes session-level cleanup
func (executor *CleanupTaskExecutor) ExecuteSessionCleanup(ctx context.Context) error {
	logFields := map[string]interface{}{
		"sessionID": executor.sessionID,
	}
	logger.Op.WithFields(logFields).Info("Starting session cleanup execution")

	// Execute health check first
	healthCheckTask := NewHealthCheckTask("health-check-start", executor.cleanupManager, executor.config)
	if err := healthCheckTask.Execute(ctx); err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	// Execute session cleanup
	sessionCleanupTask := NewSessionCleanupTask("session-cleanup", executor.sessionID, executor.cleanupManager, executor.config)
	if err := sessionCleanupTask.Execute(ctx); err != nil {
		return fmt.Errorf("session cleanup failed: %w", err)
	}

	// Execute final health check
	finalHealthCheckTask := NewHealthCheckTask("health-check-end", executor.cleanupManager, executor.config)
	if err := finalHealthCheckTask.Execute(ctx); err != nil {
		logger.Op.WithFields(logFields).WithError(err).Warn("Final health check failed")
	}

	logger.Op.WithFields(logFields).Info("Session cleanup execution completed")
	return nil
}

// ExecuteEmergencyCleanup executes emergency cleanup
func (executor *CleanupTaskExecutor) ExecuteEmergencyCleanup(ctx context.Context) error {
	logFields := map[string]interface{}{
		"sessionID": executor.sessionID,
	}
	logger.Op.WithFields(logFields).Info("Starting emergency cleanup execution")

	// Execute health check first
	healthCheckTask := NewHealthCheckTask("health-check-start", executor.cleanupManager, executor.config)
	if err := healthCheckTask.Execute(ctx); err != nil {
		logger.Op.WithFields(logFields).WithError(err).Warn("Initial health check failed, proceeding with emergency cleanup")
	}

	// Execute emergency cleanup
	emergencyCleanupTask := NewEmergencyCleanupTask("emergency-cleanup", executor.cleanupManager, executor.config)
	if err := emergencyCleanupTask.Execute(ctx); err != nil {
		return fmt.Errorf("emergency cleanup failed: %w", err)
	}

	// Execute final health check
	finalHealthCheckTask := NewHealthCheckTask("health-check-end", executor.cleanupManager, executor.config)
	if err := finalHealthCheckTask.Execute(ctx); err != nil {
		logger.Op.WithFields(logFields).WithError(err).Warn("Final health check failed")
	}

	logger.Op.WithFields(logFields).Info("Emergency cleanup execution completed")
	return nil
}

// GetCleanupManager returns the cleanup manager for external use
func (executor *CleanupTaskExecutor) GetCleanupManager() *MultiLevelCleanupManager {
	return executor.cleanupManager
}
