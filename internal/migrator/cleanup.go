package migrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// CleanupSnapshots performs legacy cleanup for backward compatibility
func CleanupSnapshots(ctx context.Context, config *Config, gcpClient *gcp.Clients, results []MigrationResult) error {
	logger.User.Info("--- Phase 3: Cleanup ---")
	logger.User.Infof("Searching for snapshots with label '%s=%s' in project %s for cleanup...",
		gcp.MANAGED_BY_KEY, gcp.MANAGED_BY_VALUE, config.ProjectID)

	snapshotsToDelete, err := gcpClient.SnapshotClient.ListSnapshotsByLabel(ctx, config.ProjectID, gcp.MANAGED_BY_KEY, gcp.MANAGED_BY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to list snapshots for cleanup: %w", err)
	}

	if len(snapshotsToDelete) == 0 {
		logger.User.Info("No snapshots found with the cleanup label.")
		logger.User.Info("--- Cleanup Phase Complete ---")
		return nil
	}

	logger.User.Infof("Found %d snapshot(s) to cleanup:", len(snapshotsToDelete))
	snapshotMap := make(map[string]bool)
	for _, snap := range snapshotsToDelete {
		logger.User.Infof("  - %s", snap.GetName())
		snapshotMap[snap.GetName()] = false
	}

	var wg sync.WaitGroup
	concurrencyLimit := config.Concurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 200 {
		concurrencyLimit = 10
	}
	semaphore := make(chan struct{}, concurrencyLimit)
	deleteErrors := &sync.Map{}

	logger.User.Infof("Starting cleanup for %d snapshot(s) with concurrency limit of %d...", len(snapshotsToDelete), concurrencyLimit)

	for _, snap := range snapshotsToDelete {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			logFields := map[string]interface{}{"snapshot": snapshotName, "project": config.ProjectID}
			logger.Op.WithFields(logFields).Info("Attempting to delete snapshot...")
			err := gcpClient.SnapshotClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if err != nil {
				logger.Op.WithFields(logFields).WithError(err).Warn("Failed to delete snapshot during cleanup")
				deleteErrors.Store(snapshotName, err)
			} else {
				logger.Op.WithFields(logFields).Info("Snapshot deleted successfully during cleanup.")
				snapshotMap[snapshotName] = true
			}
		}(snap.GetName())
	}

	wg.Wait()

	cleanupFailedCount := 0
	for i := range results {
		if deleted, exists := snapshotMap[results[i].SnapshotName]; exists {
			results[i].SnapshotCleaned = deleted
			if !deleted {
				cleanupFailedCount++
				if errVal, ok := deleteErrors.Load(results[i].SnapshotName); ok {
					if e, ok := errVal.(error); ok {
						results[i].ErrorMessage += fmt.Sprintf(" | Cleanup Failed: %v", e)
					}
				}
			}
		}
	}

	if cleanupFailedCount > 0 {
		logger.User.Warnf("%d snapshot(s) failed to delete during cleanup. Manual cleanup may be required.", cleanupFailedCount)
	} else {
		logger.User.Info("Snapshot cleanup completed successfully.")
	}

	logger.User.Info("--- Cleanup Phase Complete ---")
	return nil
}

// CleanupSnapshotsWithManager performs enhanced cleanup using the multi-level cleanup manager
func CleanupSnapshotsWithManager(ctx context.Context, config *Config, gcpClient *gcp.Clients, sessionID string, results []MigrationResult) error {
	logger.User.Info("--- Phase 3: Enhanced Cleanup ---")
	
	// Create cleanup manager with default strategy
	strategy := DefaultCleanupStrategy()
	cleanupManager := NewMultiLevelCleanupManager(config, gcpClient, strategy, sessionID)
	
	// Perform session-level cleanup
	result := cleanupManager.CleanupSessionSnapshots(ctx)
	
	// Update results with cleanup status
	for i := range results {
		// Try to find this snapshot in the cleanup results
		snapshotCleaned := false
		var cleanupError error
		
		for j, failedSnapshot := range result.SnapshotsFailed {
			if failedSnapshot == results[i].SnapshotName {
				cleanupError = result.Errors[j]
				break
			}
		}
		
		// If not in failed list, check if it was among the deleted ones
		if cleanupError == nil {
			snapshotCleaned = result.SnapshotsDeleted > 0
		}
		
		results[i].SnapshotCleaned = snapshotCleaned
		if cleanupError != nil {
			if results[i].ErrorMessage != "" {
				results[i].ErrorMessage += fmt.Sprintf(" | Cleanup Failed: %v", cleanupError)
			} else {
				results[i].ErrorMessage = fmt.Sprintf("Cleanup Failed: %v", cleanupError)
			}
		}
	}
	
	// Log cleanup summary
	logger.User.Infof("Enhanced cleanup completed: %d found, %d deleted, %d failed (duration: %v)", 
		result.SnapshotsFound, result.SnapshotsDeleted, len(result.SnapshotsFailed), result.Duration)
	
	if len(result.SnapshotsFailed) > 0 {
		logger.User.Warnf("%d snapshot(s) failed to delete during enhanced cleanup. Manual cleanup may be required.", len(result.SnapshotsFailed))
		return fmt.Errorf("enhanced cleanup completed with %d failures", len(result.SnapshotsFailed))
	}
	
	logger.User.Info("--- Enhanced Cleanup Phase Complete ---")
	return nil
}

// PerformEmergencyCleanup performs emergency cleanup of all expired snapshots
func PerformEmergencyCleanup(ctx context.Context, config *Config, gcpClient *gcp.Clients) error {
	logger.User.Info("--- Emergency Cleanup: Expired Snapshots ---")
	
	// Create cleanup manager for emergency cleanup
	strategy := DefaultCleanupStrategy()
	// Use a temporary session ID for emergency cleanup
	sessionID := gcp.GenerateSessionID()
	cleanupManager := NewMultiLevelCleanupManager(config, gcpClient, strategy, sessionID)
	
	// Perform emergency cleanup
	result := cleanupManager.CleanupExpiredSnapshots(ctx)
	
	// Log cleanup summary
	logger.User.Infof("Emergency cleanup completed: %d found, %d deleted, %d failed (duration: %v)", 
		result.SnapshotsFound, result.SnapshotsDeleted, len(result.SnapshotsFailed), result.Duration)
	
	if len(result.SnapshotsFailed) > 0 {
		logger.User.Warnf("%d expired snapshot(s) failed to delete during emergency cleanup.", len(result.SnapshotsFailed))
		// Don't return error for emergency cleanup - it's best effort
	}
	
	logger.User.Info("--- Emergency Cleanup Complete ---")
	return nil
}
