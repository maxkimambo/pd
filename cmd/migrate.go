package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate detached persistent disks, or disks attached to GCE instances to a new disk type",
	Long: `Migrate detached persistent disks, or disks attached to GCE instances to a new disk type by creating snapshots and recreating disks.
This command supports both detached disks and disks attached to GCE instances.`,
}

func init() {
	migrateCmd.PersistentFlags().StringP("project", "p", "", "The Google Cloud project ID where the resources are located (required)")
	migrateCmd.PersistentFlags().Int64("throughput", 140, "Throughput in MB/s for new disks (140-5000 MB/s)")
	migrateCmd.PersistentFlags().Int64("iops", 3000, "IOPS for new disks (3000-350000)")
	migrateCmd.PersistentFlags().StringP("storage-pool-id", "s", "", "Storage pool ID to use for the new disks (optional)")

	if err := migrateCmd.MarkPersistentFlagRequired("project"); err != nil {
		// This should not happen, but log if it does
		panic(fmt.Sprintf("Failed to mark project as required: %v", err))
	}

	migrateCmd.AddCommand(diskCmd)
	migrateCmd.AddCommand(computeCmd)
}
