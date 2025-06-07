package cmd

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate detached persistent disks, or disks attached to GCE instances to a new disk type",
	Long: `Migrate detached persistent disks, or disks attached to GCE instances to a new disk type by creating snapshots and recreating disks.
This command supports both detached disks and disks attached to GCE instances.`,
}

func init() {
	migrateCmd.AddCommand(diskCmd)
	migrateCmd.AddCommand(computeCmd)
}
