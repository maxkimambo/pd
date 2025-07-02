package tasks

import (
	"context"
	"fmt"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// CleanupSnapshotsTask deletes snapshots created during migration
type CleanupSnapshotsTask struct {
	BaseTask
}

// NewCleanupSnapshotsTask creates a new CleanupSnapshotsTask
func NewCleanupSnapshotsTask() taskmanager.TaskFunc {
	task := &CleanupSnapshotsTask{
		BaseTask: BaseTask{Name: "cleanup_snapshots"},
	}
	return task.Execute
}

// Execute deletes snapshots created during the migration process
func (t *CleanupSnapshotsTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// Check verification status
		verificationStatus, ok := shared.Get("verification_status")
		if !ok {
			logger.Warn("Verification status not found, skipping snapshot cleanup")
			return nil
		}

		if verificationStatus != "passed" {
			logger.Warn("Migration verification did not pass, skipping snapshot cleanup for safety")
			shared.Set("cleanup_status", "skipped - verification failed")
			return nil
		}

		// Get snapshot map
		snapshotMapData, ok := shared.Get("snapshot_map")
		if !ok {
			logger.Info("No snapshots found to clean up")
			shared.Set("cleanup_status", "skipped - no snapshots")
			return nil
		}

		snapshotMap, ok := snapshotMapData.(map[string]string)
		if !ok {
			return fmt.Errorf("invalid snapshot_map type in shared context")
		}

		if len(snapshotMap) == 0 {
			logger.Info("No snapshots to clean up")
			shared.Set("cleanup_status", "skipped - empty snapshot map")
			return nil
		}

		// Get GCP client
		gcpClient, err := getGCPClient(shared)
		if err != nil {
			return err
		}

		// Get project ID
		configData, err := getConfig(shared)
		if err != nil {
			return err
		}

		projectID := ""
		if config, ok := configData.(map[string]interface{}); ok {
			if p, ok := config["ProjectID"].(string); ok {
				projectID = p
			}
		}

		if projectID == "" {
			return fmt.Errorf("project ID not found in config")
		}

		logger.Infof("Cleaning up %d snapshot(s) created during migration", len(snapshotMap))

		// Track deleted snapshots
		deletedSnapshots := []string{}
		var deleteErrors []error

		// Delete each snapshot
		for diskName, snapshotName := range snapshotMap {
			logger.Infof("Deleting snapshot %s (created from disk %s)", snapshotName, diskName)

			err := gcpClient.SnapshotClient.DeleteSnapshot(ctx, projectID, snapshotName)
			if err != nil {
				logger.Errorf("Failed to delete snapshot %s: %v", snapshotName, err)
				deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete snapshot %s: %w", snapshotName, err))
				continue
			}

			deletedSnapshots = append(deletedSnapshots, snapshotName)
			logger.Successf("Snapshot %s deleted successfully", snapshotName)
		}

		// Store cleanup results
		shared.Set("deleted_snapshots", deletedSnapshots)

		if len(deleteErrors) > 0 {
			shared.Set("cleanup_status", fmt.Sprintf("partial - deleted %d/%d snapshots",
				len(deletedSnapshots), len(snapshotMap)))
			logger.Warnf("Snapshot cleanup completed with %d error(s)", len(deleteErrors))
			// Don't fail the task for cleanup errors
			return nil
		}

		shared.Set("cleanup_status", "completed")
		logger.Successf("All snapshots cleaned up successfully")

		return nil
	})
}
