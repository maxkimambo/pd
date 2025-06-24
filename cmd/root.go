package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	projectID string
	debug     bool
	verbose   bool
	jsonLogs  bool
	quiet     bool
	version   = "v0.1.0"

	rootCmd = &cobra.Command{
		Use:   "pd",
		Short: "Migrate Google Cloud persistent disks to new disk types",
		Long: `pd is a CLI tool for migrating Google Cloud persistent disks to new disk types.

Supports migration of:
• Detached persistent disks 
• Disks attached to Compute Engine instances (with minimal downtime)
		[Detached disks are those not currently attached to any VM instance.]

Key Features:
• Task-based orchestration with dependency management
• Parallel processing with configurable concurrency
• Automatic snapshot creation and cleanup
• KMS encryption support for snapshots
• Comprehensive validation and error handling
• Detailed progress logging and reporting

Prerequisites:
• GCP project with Compute Engine API enabled
• IAM permissions: compute.disks.*, compute.snapshots.*, compute.instances.*
• Sufficient quota for snapshots and disk operations

Safety Recommendations:
• Test on non-production resources first
• Ensure recent backups exist before migration
• Verify disks are not in async replication state

For more information and examples, visit: https://github.com/maxkimambo/pd`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if debug {
				logrus.SetLevel(logrus.DebugLevel)
				logrus.Debug("Debug logging enabled")
			} else {
				logrus.SetLevel(logrus.InfoLevel)
			}
			logrus.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "GCP Project ID where resources are located (required)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging with detailed operation traces")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging with progress details")
	rootCmd.PersistentFlags().BoolVar(&jsonLogs, "json", false, "Output logs in JSON format for structured processing")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output except errors and final results")

	rootCmd.AddCommand(migrateCmd)
}
