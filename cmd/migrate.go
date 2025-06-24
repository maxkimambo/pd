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
	// Register new DAG-based commands as default
	migrateCmd.AddCommand(diskCmd)
	migrateCmd.AddCommand(computeCmd)

	// Register legacy commands as hidden fallbacks
	diskCmdLegacy.Hidden = true
	computeCmdLegacy.Hidden = true
	diskCmdLegacy.Use = "disk-legacy"
	computeCmdLegacy.Use = "compute-legacy"
	migrateCmd.AddCommand(diskCmdLegacy)
	migrateCmd.AddCommand(computeCmdLegacy)
}
