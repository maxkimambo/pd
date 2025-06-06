package migrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/gcp"

	"github.com/sirupsen/logrus"
)

const snapshotCleanupLabelKey = "managed-by"
const snapshotCleanupLabelValue = "pd-migrate"

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
	snapshotMap := make(map[string]bool)
	for _, snap := range snapshotsToDelete {
		logrus.Infof("  - %s", snap.GetName())
		snapshotMap[snap.GetName()] = false
	}

	var wg sync.WaitGroup
	concurrencyLimit := config.Concurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 200 {
		concurrencyLimit = 10
	}
	semaphore := make(chan struct{}, concurrencyLimit)
	deleteErrors := &sync.Map{}

	logrus.Infof("Starting cleanup for %d snapshot(s) with concurrency limit of %d...", len(snapshotsToDelete), concurrencyLimit)

	for _, snap := range snapshotsToDelete {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			logFields := logrus.Fields{"snapshot": snapshotName, "project": config.ProjectID}
			logrus.WithFields(logFields).Info("Attempting to delete snapshot...")
			err := gcpClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if err != nil {
				logrus.WithFields(logFields).WithError(err).Warn("Failed to delete snapshot during cleanup")
				deleteErrors.Store(snapshotName, err)
			} else {
				logrus.WithFields(logFields).Info("Snapshot deleted successfully during cleanup.")
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
		logrus.Warnf("%d snapshot(s) failed to delete during cleanup. Manual cleanup may be required.", cleanupFailedCount)
	} else {
		logrus.Info("Snapshot cleanup completed successfully.")
	}

	logrus.Info("--- Cleanup Phase Complete ---")
	return nil
}
