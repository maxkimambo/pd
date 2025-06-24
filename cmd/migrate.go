package cmd

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate persistent disks to new disk types with automated orchestration",
	Long: `Migrate Google Cloud persistent disks to new disk types using automated task orchestration.

This command provides two migration modes:
• 'disk'    - Migrate detached persistent disks (bulk operations)
• 'compute' - Migrate disks attached to Compute Engine instances

Migration Process:
1. Discovery and validation of target resources
2. Snapshot creation with optional KMS encryption
3. Disk recreation with new disk type and specifications
4. Automatic cleanup of intermediate snapshots

⚠️  IMPORTANT SAFETY NOTICE:
• This tool performs destructive operations on persistent disks
• Always ensure recent backups exist before proceeding
• Test migrations on non-production resources first
• Monitor GCP Console during operations for any issues
• Verify application compatibility with new disk types

The orchestration engine provides dependency management, parallel processing,
and automatic error recovery to minimize downtime and ensure data integrity.`,
}

func init() {
	// Register DAG-based commands
	migrateCmd.AddCommand(diskCmd)
	migrateCmd.AddCommand(computeCmd)
}
