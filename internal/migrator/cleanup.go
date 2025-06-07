package migrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

const snapshotCleanupLabelKey = "managed-by"
const snapshotCleanupLabelValue = "pd-migrate"

func CleanupSnapshots(ctx context.Context, config *Config, gcpClient *gcp.Clients, results []MigrationResult) error {
	logger.User.Info("--- Phase 3: Cleanup ---")
	logger.User.Infof("Searching for snapshots with label '%s=%s' in project %s for cleanup...",
		snapshotCleanupLabelKey, snapshotCleanupLabelValue, config.ProjectID)

	snapshotsToDelete, err := gcpClient.SnapshotClient.ListSnapshotsByLabel(ctx, config.ProjectID, snapshotCleanupLabelKey, snapshotCleanupLabelValue)
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
