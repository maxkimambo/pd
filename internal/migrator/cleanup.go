package migrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
)

func CleanupSnapshots(ctx context.Context, config *Config, gcpClient *gcp.Clients, results []MigrationResult) error {
	logger.Starting("🧽 Cleanup Phase")
	logger.Infof("Searching for snapshots with label '%s=%s' in project %s for cleanup...",
		gcp.MANAGED_BY_KEY, gcp.MANAGED_BY_VALUE, config.ProjectID)

	snapshotsToDelete, err := gcpClient.SnapshotClient.ListSnapshotsByLabel(ctx, config.ProjectID, gcp.MANAGED_BY_KEY, gcp.MANAGED_BY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to list snapshots for cleanup: %w", err)
	}

	if len(snapshotsToDelete) == 0 {
		logger.Info("No snapshots found with the cleanup label.")
		logger.Success("Cleanup phase completed")
		return nil
	}

	logger.Infof("Found %d snapshot(s) to cleanup:", len(snapshotsToDelete))
	snapshotMap := make(map[string]bool)
	snapshotNames := make([]string, 0, len(snapshotsToDelete))
	for _, snap := range snapshotsToDelete {
		logger.Infof("  - %s", snap.GetName())
		snapshotMap[snap.GetName()] = false
		snapshotNames = append(snapshotNames, snap.GetName())
	}

	// Prompt for confirmation before deleting snapshots
	confirmed, err := utils.PromptForMultipleItems(
		config.AutoApproveAll,
		"delete snapshots",
		snapshotNames,
	)
	if err != nil {
		return fmt.Errorf("failed to get user confirmation: %w", err)
	}
	if !confirmed {
		logger.Info("Snapshot cleanup cancelled by user.")
		logger.Success("Cleanup phase completed")
		return nil
	}

	var wg sync.WaitGroup
	concurrencyLimit := config.Concurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 200 {
		concurrencyLimit = 10
	}
	semaphore := make(chan struct{}, concurrencyLimit)
	deleteErrors := &sync.Map{}

	logger.Infof("Starting cleanup for %d snapshot(s) with concurrency limit of %d...", len(snapshotsToDelete), concurrencyLimit)

	for _, snap := range snapshotsToDelete {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			logFields := map[string]interface{}{"snapshot": snapshotName, "project": config.ProjectID}
			logger.WithFieldsMap(logFields).Info("Attempting to delete snapshot...")
			err := gcpClient.SnapshotClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if err != nil {
				logger.WithFieldsMap(logFields).WithError(err).Warn("Failed to delete snapshot during cleanup")
				deleteErrors.Store(snapshotName, err)
			} else {
				logger.WithFieldsMap(logFields).Info("Snapshot deleted successfully during cleanup.")
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
		logger.Warnf("%d snapshot(s) failed to delete during cleanup. Manual cleanup may be required.", cleanupFailedCount)
	} else {
		logger.Info("Snapshot cleanup completed successfully.")
	}

	logger.Success("Cleanup phase completed")
	return nil
}
