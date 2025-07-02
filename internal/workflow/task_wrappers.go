package workflow

import (
	"context"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// createSnapshotTask creates a task that calls the existing SnapshotInstanceDisks function
func createSnapshotTask(config *migrator.Config) taskmanager.TaskFunc {
	return func(ctx context.Context, shared *taskmanager.SharedContext) error {
		// Get required data from shared context
		inst, _ := shared.Get("instance")
		instance := inst.(*computepb.Instance)

		gcpClient, _ := shared.Get("gcp_client")
		client := gcpClient.(*gcp.Clients)

		// Call the existing function
		err := migrator.SnapshotInstanceDisks(ctx, config, instance, client)
		if err != nil {
			return err
		}

		// Set completion status
		shared.Set("snapshot_status", "completed")
		return nil
	}
}

// createMigrateTask creates a task that calls the existing MigrateInstanceNonBootDisks function
func createMigrateTask(config *migrator.Config) taskmanager.TaskFunc {
	return func(ctx context.Context, shared *taskmanager.SharedContext) error {
		// Get required data from shared context
		inst, _ := shared.Get("instance")
		instance := inst.(*computepb.Instance)

		gcpClient, _ := shared.Get("gcp_client")
		client := gcpClient.(*gcp.Clients)

		// Call the existing function
		results, err := migrator.MigrateInstanceNonBootDisks(ctx, config, instance, client)
		if err != nil {
			return err
		}

		// Store results in shared context
		shared.Set("migration_results", results)

		// Build migrated disks map
		migratedDisks := make(map[string]string)
		for _, result := range results {
			if result.Status == "Success" || result.Status == "Completed" {
				migratedDisks[result.DiskName] = result.NewDiskName
			}
		}
		shared.Set("migrated_disks", migratedDisks)

		return nil
	}
}
