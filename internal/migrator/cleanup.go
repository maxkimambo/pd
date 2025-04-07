package migrator

import (
	"context"
	"fmt"
	"gcp-disk-migrator/internal/gcp"
	"sync"

	"github.com/sirupsen/logrus"
)

const snapshotCleanupLabelKey = "managed-by"
const snapshotCleanupLabelValue = "pd-migrate"

// CleanupSnapshots performs Phase 3: Deleting snapshots created by this tool.
// It also updates the SnapshotCleaned status in the results.
func CleanupSnapshots(ctx context.Context, config *Config, gcpClient *gcp.Clients, results []MigrationResult) error {
	logrus.Info("--- Phase 3: Cleanup ---")
	logrus.Infof("Searching for snapshots with label '%s=%s' in project %s for cleanup...",
		snapshotCleanupLabelKey, snapshotCleanupLabelValue, config.ProjectID)

	snapshotsToDelete, err := gcpClient.ListSnapshotsByLabel(ctx, config.ProjectID, snapshotCleanupLabelKey, snapshotCleanupLabelValue)
	if err != nil {
		return fmt.Errorf("failed to list snapshots for cleanup: %w", err)
	}

	if len(snapshotsToDelete) == 0 {
		logrus.Info("No snapshots found with the cleanup label.")
		logrus.Info("--- Cleanup Phase Complete ---")
		return nil
	}

	logrus.Infof("Found %d snapshot(s) to cleanup:", len(snapshotsToDelete))
	snapshotMap := make(map[string]bool) // To track which snapshots were attempted/deleted
	for _, snap := range snapshotsToDelete {
		logrus.Infof("  - %s", snap.GetName())
		snapshotMap[snap.GetName()] = false // Mark as not yet deleted
	}

	var wg sync.WaitGroup
	// Use a semaphore for cleanup concurrency, potentially different limit? Using migration limit for now.
	concurrencyLimit := config.MaxConcurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 200 {
		concurrencyLimit = 10 // Default
	}
	semaphore := make(chan struct{}, concurrencyLimit)
	deleteErrors := &sync.Map{} // Store errors keyed by snapshot name

	logrus.Infof("Starting cleanup for %d snapshot(s) with concurrency limit %d...", len(snapshotsToDelete), concurrencyLimit)

	for _, snap := range snapshotsToDelete {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			logFields := logrus.Fields{"snapshot": snapshotName, "project": config.ProjectID}
			logrus.WithFields(logFields).Info("Attempting to delete snapshot...")
			err := gcpClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if err != nil {
				logrus.WithFields(logFields).WithError(err).Warn("Failed to delete snapshot during cleanup")
				deleteErrors.Store(snapshotName, err) // Record error
			} else {
				logrus.WithFields(logFields).Info("Snapshot deleted successfully during cleanup.")
				snapshotMap[snapshotName] = true // Mark as deleted
			}
		}(snap.GetName())
	}

	wg.Wait()

	// Update results based on cleanup success
	cleanupFailedCount := 0
	for i := range results {
		if deleted, exists := snapshotMap[results[i].SnapshotName]; exists {
			results[i].SnapshotCleaned = deleted
			if !deleted {
				cleanupFailedCount++
				// Add cleanup error message if available
				if errVal, ok := deleteErrors.Load(results[i].SnapshotName); ok {
					if e, ok := errVal.(error); ok {
						results[i].ErrorMessage += fmt.Sprintf(" | Cleanup Failed: %v", e)
					}
				}
			}
		}
	}

	if cleanupFailedCount > 0 {
		logrus.Warnf("%d snapshot(s) failed to delete during cleanup. Manual cleanup may be required.", cleanupFailedCount)
	} else {
		logrus.Info("Snapshot cleanup completed successfully.")
	}

	logrus.Info("--- Cleanup Phase Complete ---")
	return nil // Return nil even if some cleanups failed, log indicates issues
}
