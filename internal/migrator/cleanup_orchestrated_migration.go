package migrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// CleanupOrchestrator manages cleanup operations within the task execution framework
type CleanupOrchestrator struct {
	config           *Config
	gcpClients       *gcp.Clients
	sessionID        string
	cleanupExecutor  *CleanupTaskExecutor
	activeSnapshots  map[string]string   // snapshotName -> taskID mapping
	taskSnapshots    map[string][]string // taskID -> []snapshotName mapping
	cleanupScheduled bool
	mu               sync.RWMutex
}

// NewCleanupOrchestrator creates a new cleanup orchestrator
func NewCleanupOrchestrator(config *Config, gcpClients *gcp.Clients, sessionID string) *CleanupOrchestrator {
	return &CleanupOrchestrator{
		config:          config,
		gcpClients:      gcpClients,
		sessionID:       sessionID,
		cleanupExecutor: NewCleanupTaskExecutor(config, gcpClients, sessionID),
		activeSnapshots: make(map[string]string),
		taskSnapshots:   make(map[string][]string),
	}
}

// RegisterSnapshot registers a snapshot for cleanup tracking
func (co *CleanupOrchestrator) RegisterSnapshot(taskID, snapshotName string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	co.activeSnapshots[snapshotName] = taskID

	if co.taskSnapshots[taskID] == nil {
		co.taskSnapshots[taskID] = make([]string, 0)
	}
	co.taskSnapshots[taskID] = append(co.taskSnapshots[taskID], snapshotName)

	// Also register with the cleanup manager
	co.cleanupExecutor.GetCleanupManager().RegisterSnapshot(snapshotName)

	logFields := map[string]interface{}{
		"sessionID":    co.sessionID,
		"taskID":       taskID,
		"snapshotName": snapshotName,
		"totalActive":  len(co.activeSnapshots),
	}
	logger.Op.WithFields(logFields).Debug("Registered snapshot for cleanup orchestration")
}

// UnregisterSnapshot removes a snapshot from cleanup tracking
func (co *CleanupOrchestrator) UnregisterSnapshot(snapshotName string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	taskID, exists := co.activeSnapshots[snapshotName]
	if !exists {
		return
	}

	delete(co.activeSnapshots, snapshotName)

	// Remove from task snapshots
	if snapshots, taskExists := co.taskSnapshots[taskID]; taskExists {
		for i, s := range snapshots {
			if s == snapshotName {
				co.taskSnapshots[taskID] = append(snapshots[:i], snapshots[i+1:]...)
				break
			}
		}

		// Remove task entry if no snapshots left
		if len(co.taskSnapshots[taskID]) == 0 {
			delete(co.taskSnapshots, taskID)
		}
	}

	// Also unregister with the cleanup manager
	co.cleanupExecutor.GetCleanupManager().UnregisterSnapshot(snapshotName)

	logFields := map[string]interface{}{
		"sessionID":    co.sessionID,
		"taskID":       taskID,
		"snapshotName": snapshotName,
		"totalActive":  len(co.activeSnapshots),
	}
	logger.Op.WithFields(logFields).Debug("Unregistered snapshot from cleanup orchestration")
}

// ExecuteTaskCleanup executes cleanup for completed tasks using task execution
func (co *CleanupOrchestrator) ExecuteTaskCleanup(ctx context.Context, completedTaskID string) error {
	co.mu.RLock()
	snapshots, exists := co.taskSnapshots[completedTaskID]
	co.mu.RUnlock()

	if !exists || len(snapshots) == 0 {
		logFields := map[string]interface{}{
			"sessionID": co.sessionID,
			"taskID":    completedTaskID,
		}
		logger.Op.WithFields(logFields).Debug("No snapshots to clean up for completed task")
		return nil
	}

	logFields := map[string]interface{}{
		"sessionID":     co.sessionID,
		"taskID":        completedTaskID,
		"snapshotCount": len(snapshots),
		"snapshots":     snapshots,
	}
	logger.Op.WithFields(logFields).Info("Starting task cleanup using task execution")

	// Build cleanup task map for this specific task
	taskCleanupMap := map[string][]string{
		completedTaskID: snapshots,
	}

	// Execute the cleanup using the task executor
	startTime := time.Now()
	err := co.cleanupExecutor.ExecuteTaskCleanup(ctx, taskCleanupMap)
	executionTime := time.Since(startTime)

	if err != nil {
		logFields["error"] = err.Error()
		logFields["executionTime"] = executionTime
		logger.Op.WithFields(logFields).Error("Task cleanup execution failed")
		return fmt.Errorf("task cleanup execution failed: %w", err)
	}

	// Remove cleaned snapshots from tracking
	for _, snapshotName := range snapshots {
		co.UnregisterSnapshot(snapshotName)
	}

	logFields["executionTime"] = executionTime
	logger.Op.WithFields(logFields).Info("Task cleanup execution completed successfully")

	return nil
}

// ExecuteSessionCleanup executes session-level cleanup using task execution
func (co *CleanupOrchestrator) ExecuteSessionCleanup(ctx context.Context) error {
	logFields := map[string]interface{}{
		"sessionID": co.sessionID,
	}
	logger.Op.WithFields(logFields).Info("Starting session cleanup using task execution")

	// Execute the cleanup using the task executor
	startTime := time.Now()
	err := co.cleanupExecutor.ExecuteSessionCleanup(ctx)
	executionTime := time.Since(startTime)

	if err != nil {
		logFields["error"] = err.Error()
		logFields["executionTime"] = executionTime
		logger.Op.WithFields(logFields).Error("Session cleanup execution failed")
		return fmt.Errorf("session cleanup execution failed: %w", err)
	}

	// Clear all tracked snapshots since session cleanup should have handled them
	co.mu.Lock()
	co.activeSnapshots = make(map[string]string)
	co.taskSnapshots = make(map[string][]string)
	co.mu.Unlock()

	logFields["executionTime"] = executionTime
	logger.Op.WithFields(logFields).Info("Session cleanup execution completed successfully")

	return nil
}

// ExecuteEmergencyCleanup executes emergency cleanup using task execution
func (co *CleanupOrchestrator) ExecuteEmergencyCleanup(ctx context.Context) error {
	logFields := map[string]interface{}{
		"sessionID": co.sessionID,
	}
	logger.Op.WithFields(logFields).Info("Starting emergency cleanup using task execution")

	// Execute the cleanup using the task executor
	startTime := time.Now()
	err := co.cleanupExecutor.ExecuteEmergencyCleanup(ctx)
	executionTime := time.Since(startTime)

	// Emergency cleanup is more tolerant of failures
	if err != nil {
		logFields["error"] = err.Error()
		logFields["executionTime"] = executionTime
		logger.Op.WithFields(logFields).Warn("Emergency cleanup execution had some failures (tolerated)")
	} else {
		logFields["executionTime"] = executionTime
		logger.Op.WithFields(logFields).Info("Emergency cleanup execution completed successfully")
	}

	return nil
}

// ScheduleCleanup schedules cleanup operations in the background
func (co *CleanupOrchestrator) ScheduleCleanup(ctx context.Context, interval time.Duration) {
	co.mu.Lock()
	if co.cleanupScheduled {
		co.mu.Unlock()
		return
	}
	co.cleanupScheduled = true
	co.mu.Unlock()

	logFields := map[string]interface{}{
		"sessionID": co.sessionID,
		"interval":  interval,
	}
	logger.Op.WithFields(logFields).Info("Starting scheduled cleanup operations")

	// Health monitoring removed - using simpler cleanup approach

	// Run periodic cleanup checks
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Op.WithFields(logFields).Info("Scheduled cleanup stopped due to context cancellation")
				return
			case <-ticker.C:
				co.performScheduledMaintenance(ctx)
			}
		}
	}()
}

// performScheduledMaintenance performs periodic maintenance tasks
func (co *CleanupOrchestrator) performScheduledMaintenance(ctx context.Context) {
	logFields := map[string]interface{}{
		"sessionID": co.sessionID,
	}

	// Simple health check - just log that maintenance is running
	logger.Op.WithFields(logFields).Debug("Performing scheduled cleanup maintenance")

	// Run emergency cleanup if we have too many active snapshots
	co.mu.RLock()
	activeCount := len(co.activeSnapshots)
	co.mu.RUnlock()

	// Threshold for triggering emergency cleanup
	emergencyThreshold := 100
	if co.config.Concurrency > 0 {
		emergencyThreshold = co.config.Concurrency * 10 // 10x concurrency setting
	}

	if activeCount > emergencyThreshold {
		logFields["activeSnapshots"] = activeCount
		logFields["threshold"] = emergencyThreshold
		logger.Op.WithFields(logFields).Warn("Active snapshot count exceeds threshold, triggering emergency cleanup")

		emergencyCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		if err := co.ExecuteEmergencyCleanup(emergencyCtx); err != nil {
			logger.Op.WithFields(logFields).WithError(err).Error("Scheduled emergency cleanup failed")
		}
	}
}

// GetActiveSnapshotCount returns the number of currently tracked snapshots
func (co *CleanupOrchestrator) GetActiveSnapshotCount() int {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return len(co.activeSnapshots)
}

// GetTaskSnapshotCount returns the number of snapshots for a specific task
func (co *CleanupOrchestrator) GetTaskSnapshotCount(taskID string) int {
	co.mu.RLock()
	defer co.mu.RUnlock()
	if snapshots, exists := co.taskSnapshots[taskID]; exists {
		return len(snapshots)
	}
	return 0
}

// Close performs cleanup of the orchestrator
func (co *CleanupOrchestrator) Close() error {
	co.mu.Lock()
	defer co.mu.Unlock()

	logFields := map[string]interface{}{
		"sessionID":          co.sessionID,
		"remainingSnapshots": len(co.activeSnapshots),
	}

	if len(co.activeSnapshots) > 0 {
		logger.Op.WithFields(logFields).Warn("Cleanup orchestrator closing with remaining active snapshots")
	}

	logger.Op.WithFields(logFields).Info("Cleanup orchestrator closed")
	return nil
}

// OrchestatedMigrationOrchestrator extends MigrationOrchestrator with cleanup orchestration
type OrchestatedMigrationOrchestrator struct {
	*MigrationOrchestrator
	cleanupOrchestrator *CleanupOrchestrator
}

// NewOrchestatedMigrationOrchestrator creates a new migration orchestrator with cleanup orchestration
func NewOrchestatedMigrationOrchestrator(config *Config, resourceLocker *ResourceLocker, progressTracker *ProgressTracker, sessionID string) (*OrchestatedMigrationOrchestrator, error) {
	// Create base migration orchestrator
	baseOrchestrator, err := NewMigrationOrchestrator(config, resourceLocker, progressTracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create base migration orchestrator: %w", err)
	}

	// Create GCP clients for cleanup orchestrator
	ctx := context.Background()
	clients, err := gcp.NewClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP clients for cleanup orchestrator: %w", err)
	}

	// Create cleanup orchestrator
	cleanupOrchestrator := NewCleanupOrchestrator(config, clients, sessionID)

	return &OrchestatedMigrationOrchestrator{
		MigrationOrchestrator: baseOrchestrator,
		cleanupOrchestrator:   cleanupOrchestrator,
	}, nil
}

// MigrateInstanceWithCleanup performs instance migration with integrated cleanup orchestration
func (omo *OrchestatedMigrationOrchestrator) MigrateInstanceWithCleanup(ctx context.Context, job *MigrationJob) *InstanceMigrationResult {
	// Start cleanup scheduling
	cleanupCtx, cleanupCancel := context.WithCancel(ctx)
	defer cleanupCancel()

	omo.cleanupOrchestrator.ScheduleCleanup(cleanupCtx, 5*time.Minute)

	// Track any snapshots created during migration
	snapshotTracking := make(map[string]string) // snapshotName -> taskID

	// Execute base migration
	result := omo.MigrateInstance(ctx, job)

	// Extract snapshot information from migration results if available
	if result.Migration != nil {
		for i, diskResult := range result.Migration.Results {
			if diskResult.Success && diskResult.NewDiskLink != "" {
				// Create a synthetic snapshot name based on the disk migration
				taskID := fmt.Sprintf("disk-migration-%d", i)
				snapshotName := fmt.Sprintf("snapshot-%s-%d", job.Instance.GetName(), i)

				snapshotTracking[snapshotName] = taskID
				omo.cleanupOrchestrator.RegisterSnapshot(taskID, snapshotName)
			}
		}
	}

	// Perform task-level cleanup for any tracked snapshots
	if len(snapshotTracking) > 0 {
		logFields := map[string]interface{}{
			"jobID":         job.ID,
			"instanceName":  job.Instance.GetName(),
			"snapshotCount": len(snapshotTracking),
		}
		logger.Op.WithFields(logFields).Info("Starting post-migration cleanup")

		// Execute cleanup for each task
		for _, taskID := range snapshotTracking {
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := omo.cleanupOrchestrator.ExecuteTaskCleanup(cleanupCtx, taskID); err != nil {
				logger.Op.WithFields(logFields).WithError(err).Warn("Post-migration task cleanup failed")
			}
			cancel()
		}
	}

	logger.Op.WithFields(map[string]interface{}{
		"jobID":        job.ID,
		"instanceName": job.Instance.GetName(),
	}).Info("Migration completed successfully")

	return result
}

// GetCleanupOrchestrator returns the cleanup orchestrator for external access
func (omo *OrchestatedMigrationOrchestrator) GetCleanupOrchestrator() *CleanupOrchestrator {
	return omo.cleanupOrchestrator
}

// Close performs cleanup of both orchestrators
func (omo *OrchestatedMigrationOrchestrator) Close() error {
	var errs []error

	if err := omo.cleanupOrchestrator.Close(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup orchestrator close failed: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("orchestrator close failed: %v", errs)
	}

	return nil
}
