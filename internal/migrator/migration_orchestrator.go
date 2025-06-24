package migrator

import (
	"context"
	"fmt"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// InstanceMigrationResult represents the result of a complete instance migration
type InstanceMigrationResult struct {
	JobID        string
	InstanceID   string
	InstanceName string
	StartTime    time.Time
	EndTime      time.Time
	Success      bool
	Error        error
	Migration    *InstanceMigration
	// Progress tracking fields
	TotalSteps     int
	CompletedSteps int
	CurrentStep    string
	// Performance metrics
	DurationSeconds int64
	DisksProcessed  int
	DisksSuccessful int
	DisksFailed     int
	// Enhanced error information
	EnhancedError *EnhancedMigrationError
	ErrorCategory MigrationErrorCategory
	IsRetryable   bool
	RetryDelay    time.Duration
}

// MigrationOrchestrator coordinates a single instance migration
// Each orchestrator instance is self-contained and maintains its own state
type MigrationOrchestrator struct {
	// Service dependencies - each orchestrator has its own instances
	discoveryService *InstanceDiscovery
	diskMigrator     *DiskMigrator
	stateManager     *InstanceStateManager

	// Configuration
	config *Config

	// Resource locking - shared across orchestrators to prevent conflicts
	resourceLocker *ResourceLocker

	// Progress tracking - shared across orchestrators for monitoring
	progressTracker *ProgressTracker

	// Internal state for this orchestrator instance
	migrationID string
	startTime   time.Time

	// Progress tracking
	currentStep    string
	totalSteps     int
	completedSteps int
}

// NewMigrationOrchestrator creates a new orchestrator with its own service instances
// This ensures isolation between different orchestrator instances used by workers
// The resourceLocker and progressTracker should be shared across all orchestrators
func NewMigrationOrchestrator(config *Config, resourceLocker *ResourceLocker, progressTracker *ProgressTracker) (*MigrationOrchestrator, error) {
	// Create GCP clients for this orchestrator instance
	ctx := context.Background()
	clients, err := gcp.NewClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP clients for orchestrator: %w", err)
	}

	// Create isolated service instances for this orchestrator
	discoveryService := NewInstanceDiscovery(clients.ComputeClient, clients.DiskClient)
	diskMigrator := NewDiskMigrator(clients.ComputeClient, clients.DiskClient, clients.SnapshotClient, config)
	stateManager := NewInstanceStateManager()

	return &MigrationOrchestrator{
		discoveryService: discoveryService,
		diskMigrator:     diskMigrator,
		stateManager:     stateManager,
		config:           config,
		resourceLocker:   resourceLocker,
		progressTracker:  progressTracker,
		totalSteps:       6, // Discovery, Resource Locking, State Management, Migration, Verification, Cleanup
		completedSteps:   0,
		currentStep:      "initialized",
	}, nil
}

// MigrateInstance handles the complete migration of a single instance
// This method is self-contained and does not rely on shared state
// It respects context cancellation at all major phases and provides proper cleanup
func (o *MigrationOrchestrator) MigrateInstance(ctx context.Context, job *MigrationJob) *InstanceMigrationResult {
	result := &InstanceMigrationResult{
		JobID:        job.ID,
		InstanceID:   job.Instance.GetName(),
		InstanceName: job.Instance.GetName(),
		StartTime:    time.Now(),
		TotalSteps:   o.totalSteps,
		Success:      false,
	}

	// Setup cleanup function that will be called regardless of how we exit
	defer func() {
		result.EndTime = time.Now()
		result.DurationSeconds = int64(result.EndTime.Sub(result.StartTime).Seconds())

		// Always cleanup our local state on exit
		if result.Migration != nil && result.Migration.Instance != nil {
			o.stateManager.RemoveState(result.Migration.Instance)
		}

		// Cleanup progress tracking for completed job
		if o.progressTracker != nil {
			// Report final progress event
			finalEvent := &ProgressEvent{
				JobID:           result.JobID,
				InstanceID:      result.InstanceID,
				InstanceName:    result.InstanceName,
				Step:            "completed",
				Description:     "Migration completed",
				CompletedSteps:  o.totalSteps,
				TotalSteps:      o.totalSteps,
				ProgressPercent: 100.0,
				Timestamp:       time.Now(),
				ElapsedTime:     time.Duration(result.DurationSeconds) * time.Second,
				DisksProcessed:  result.DisksProcessed,
				DisksCompleted:  result.DisksSuccessful,
				Phase:           PhaseCleanup,
				Details: map[string]interface{}{
					"success":          result.Success,
					"disks_successful": result.DisksSuccessful,
					"disks_failed":     result.DisksFailed,
				},
			}
			o.progressTracker.ReportProgress(finalEvent)

			// Clean up tracking data after a delay to allow final reporting
			go func() {
				time.Sleep(5 * time.Second)
				o.progressTracker.CleanupJob(result.JobID)
			}()
		}

		// Log final result
		logFields := map[string]interface{}{
			"jobID":           job.ID,
			"instanceName":    job.Instance.GetName(),
			"durationSeconds": result.DurationSeconds,
			"disksProcessed":  result.DisksProcessed,
			"disksSuccessful": result.DisksSuccessful,
			"disksFailed":     result.DisksFailed,
			"success":         result.Success,
		}

		if result.Error != nil {
			logFields["error"] = result.Error.Error()
			logger.Op.WithFields(logFields).Error("Instance migration completed with error")
		} else {
			logger.Op.WithFields(logFields).Info("Instance migration completed successfully")
		}
	}()

	// Initialize start time for this migration
	o.startTime = time.Now()

	logger.Op.WithFields(map[string]interface{}{
		"jobID":          job.ID,
		"instanceName":   job.Instance.GetName(),
		"orchestratorID": o.migrationID,
	}).Info("Starting instance migration")

	// Step 1: Discovery and preparation
	if err := o.updateProgress(ctx, result, "discovery", "Discovering instance details"); err != nil {
		o.setResultError(result, err, PhaseDiscovery)
		return result
	}

	// Check for cancellation before expensive discovery operation
	select {
	case <-ctx.Done():
		cancelErr := NewCancelledError("Migration cancelled before discovery", PhaseDiscovery)
		o.setResultError(result, cancelErr, PhaseDiscovery)
		return result
	default:
	}

	migration, err := o.discoveryService.createInstanceMigration(ctx, job.Instance, o.config)
	if err != nil {
		result.Migration = migration
		o.setResultError(result, err, PhaseDiscovery)
		return result
	}

	result.Migration = migration
	result.DisksProcessed = len(migration.Disks)

	// Check for cancellation after discovery
	select {
	case <-ctx.Done():
		cancelErr := NewCancelledError("Migration cancelled after discovery", PhaseDiscovery)
		o.setResultError(result, cancelErr, PhaseDiscovery)
		return result
	default:
	}

	// Step 2: Acquire resource locks to prevent conflicts with other workers
	if err := o.updateProgress(ctx, result, "resource-locking", "Acquiring resource locks"); err != nil {
		o.setResultError(result, err, PhasePreparation)
		return result
	}

	resources := GetResourceIDsForInstanceMigration(migration, o.config)
	if err := o.resourceLocker.LockResources(ctx, resources, job.ID); err != nil {
		// This is likely a resource conflict, categorize it appropriately
		lockErr := NewResourceConflictError("Failed to acquire resource locks", PhasePreparation, "")
		lockErr.Cause = err
		o.setResultError(result, lockErr, PhasePreparation)
		return result
	}

	// Ensure locks are released when we exit
	defer func() {
		o.resourceLocker.UnlockResources(resources, job.ID)
		logger.Op.WithFields(map[string]interface{}{
			"jobID":        job.ID,
			"instanceName": job.Instance.GetName(),
		}).Debug("Released resource locks")
	}()

	// Step 3: Store initial state
	if err := o.updateProgress(ctx, result, "state-management", "Storing initial instance state"); err != nil {
		o.setResultError(result, err, PhasePreparation)
		return result
	}

	o.stateManager.SetState(job.Instance, migration.InitialState)

	// Step 4: Perform migration
	if err := o.updateProgress(ctx, result, "migration", "Migrating instance disks"); err != nil {
		o.setResultError(result, err, PhaseMigration)
		return result
	}

	// Check for cancellation before starting the migration process
	select {
	case <-ctx.Done():
		cancelErr := NewCancelledError("Migration cancelled before disk migration", PhaseMigration)
		o.setResultError(result, cancelErr, PhaseMigration)
		return result
	default:
	}

	// Perform the disk migration with the context - the diskMigrator should also respect context
	migrationErr := o.diskMigrator.MigrateInstanceDisks(ctx, migration)

	// Check for cancellation after migration attempt
	select {
	case <-ctx.Done():
		// If context was cancelled during migration, report it as the primary error
		cancelErr := NewCancelledError("Migration cancelled during disk migration", PhaseMigration)
		o.setResultError(result, cancelErr, PhaseMigration)
		return result
	default:
	}

	// Step 5: Verify results
	if err := o.updateProgress(ctx, result, "verification", "Verifying migration results"); err != nil {
		o.setResultError(result, err, PhaseRestore)
		return result
	}

	// Count successful and failed disks
	for _, diskResult := range migration.Results {
		if diskResult.Success {
			result.DisksSuccessful++
		} else {
			result.DisksFailed++
		}
	}

	// Step 6: Cleanup
	if err := o.updateProgress(ctx, result, "cleanup", "Cleaning up migration state"); err != nil {
		o.setResultError(result, err, PhaseCleanup)
		return result
	}

	// Check for cancellation before final cleanup
	select {
	case <-ctx.Done():
		cancelErr := NewCancelledError("Migration cancelled during cleanup", PhaseCleanup)
		o.setResultError(result, cancelErr, PhaseCleanup)
		return result
	default:
	}

	// Set final success state and handle migration errors
	if migrationErr != nil {
		o.setResultError(result, migrationErr, PhaseMigration)
	} else {
		result.Success = migration.Status == MigrationStatusCompleted
		if !result.Success {
			// Migration didn't complete successfully, but no specific error was returned
			partialErr := NewPermanentError("Migration completed with partial success", PhaseMigration, SeverityMedium)
			_ = partialErr.WithDetail("disks_processed", result.DisksProcessed)
			_ = partialErr.WithDetail("disks_successful", result.DisksSuccessful)
			_ = partialErr.WithDetail("disks_failed", result.DisksFailed)
			o.setResultError(result, partialErr, PhaseMigration)
		}
	}

	return result
}

// updateProgress updates the progress tracking and checks for context cancellation
func (o *MigrationOrchestrator) updateProgress(ctx context.Context, result *InstanceMigrationResult, step, description string) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		logger.Op.WithFields(map[string]interface{}{
			"jobID":        result.JobID,
			"instanceName": result.InstanceName,
			"currentStep":  step,
		}).Info("Migration cancelled due to context cancellation")
		return ctx.Err()
	default:
		// Continue with progress update
	}

	o.currentStep = step
	o.completedSteps++

	result.CurrentStep = description
	result.CompletedSteps = o.completedSteps

	// Calculate progress metrics
	progressPercent := float64(o.completedSteps) / float64(o.totalSteps) * 100
	elapsedTime := time.Since(o.startTime)

	// Estimate time remaining
	var estimatedTimeLeft time.Duration
	if o.completedSteps > 0 {
		averageTimePerStep := elapsedTime / time.Duration(o.completedSteps)
		remainingSteps := o.totalSteps - o.completedSteps
		estimatedTimeLeft = averageTimePerStep * time.Duration(remainingSteps)
	}

	// Create progress event
	progressEvent := &ProgressEvent{
		JobID:             result.JobID,
		InstanceID:        result.InstanceID,
		InstanceName:      result.InstanceName,
		Step:              step,
		Description:       description,
		CompletedSteps:    o.completedSteps,
		TotalSteps:        o.totalSteps,
		ProgressPercent:   progressPercent,
		Timestamp:         time.Now(),
		ElapsedTime:       elapsedTime,
		EstimatedTimeLeft: estimatedTimeLeft,
		DisksProcessed:    result.DisksProcessed,
		DisksCompleted:    result.DisksSuccessful,
		Phase:             getPhaseFromStep(step),
		Details:           make(map[string]interface{}),
	}

	// Add resource usage information (simplified for now)
	progressEvent.Details["current_step"] = step
	progressEvent.Details["description"] = description

	// Report progress to tracker
	if o.progressTracker != nil {
		o.progressTracker.ReportProgress(progressEvent)
	}

	logger.Op.WithFields(map[string]interface{}{
		"jobID":          result.JobID,
		"instanceName":   result.InstanceName,
		"step":           step,
		"description":    description,
		"completedSteps": o.completedSteps,
		"totalSteps":     o.totalSteps,
		"progress":       progressPercent,
		"elapsedTime":    elapsedTime,
		"estimatedLeft":  estimatedTimeLeft,
	}).Debug("Migration progress updated")

	return nil
}

// getPhaseFromStep maps step names to migration phases
func getPhaseFromStep(step string) MigrationPhase {
	switch step {
	case "discovery":
		return PhaseDiscovery
	case "resource-locking", "state-management":
		return PhasePreparation
	case "migration":
		return PhaseMigration
	case "verification":
		return PhaseRestore
	case "cleanup":
		return PhaseCleanup
	default:
		return PhaseDiscovery
	}
}

// GetCurrentProgress returns the current progress information
func (o *MigrationOrchestrator) GetCurrentProgress() (string, int, int, float64) {
	progress := float64(o.completedSteps) / float64(o.totalSteps) * 100
	return o.currentStep, o.completedSteps, o.totalSteps, progress
}

// setResultError sets error information on the result with enhanced error handling
func (o *MigrationOrchestrator) setResultError(result *InstanceMigrationResult, err error, phase MigrationPhase) {
	result.Error = err
	result.Success = false

	// Create or enhance the error
	var enhancedErr *EnhancedMigrationError
	if eErr, ok := err.(*EnhancedMigrationError); ok {
		enhancedErr = eErr
	} else {
		enhancedErr = CategorizeError(err, phase)
	}

	// Add job context if not already present
	if enhancedErr.JobID == "" {
		_ = enhancedErr.WithJobContext(result.JobID, result.InstanceID)
	}

	result.EnhancedError = enhancedErr
	result.ErrorCategory = enhancedErr.Category
	result.IsRetryable = enhancedErr.IsRetryable
	result.RetryDelay = enhancedErr.RetryDelay

	// Log with enhanced error information
	logger.Op.WithFields(enhancedErr.ToLogFields()).Error("Migration step failed")
}

// Reset resets the orchestrator state for a new migration
func (o *MigrationOrchestrator) Reset() {
	o.currentStep = "initialized"
	o.completedSteps = 0
	o.stateManager.Clear()
}
