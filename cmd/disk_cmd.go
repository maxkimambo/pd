package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/validation"
	"github.com/spf13/cobra"
)


var diskCmd = &cobra.Command{
	Use:   "disk",
	Short: "Migrate detached persistent disks to a new disk type",
	Long: `Performs bulk migration of detached Google Cloud persistent disks.

Identifies detached disks based on location (zone or region) and optional label filter.
Migrates disks by:
1. Creating a snapshot (with optional KMS encryption)
2. Deleting the original disk (optional, controlled by --retain-name)
3. Recreating the disk from the snapshot with the target type
4. Cleaning up snapshots afterwards

Examples:
  pd migrate disk --project my-gcp-project --zone us-central1-a --target-disk-type pd-ssd
  pd migrate disk --project my-gcp-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging --concurrency 20
  pd migrate disk --project my-gcp-project --region us-central1 --target-disk-type hyperdisk-balanced --auto-approve --retain-name=false
`,
	PreRunE: validateDiskCmdFlags,
	RunE:    runConvert,
}

func init() {
	diskCmd.Flags().StringP("target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, pd-balanced, hyperdisk-balanced) (required)")
	diskCmd.Flags().String("label", "", "Label filter in key=value format (optional)")
	diskCmd.Flags().String("kms-key", "", "KMS key name for snapshot encryption (optional)")
	diskCmd.Flags().String("kms-keyring", "", "KMS keyring name (required if kms-key is set)")
	diskCmd.Flags().String("kms-location", "", "KMS key location (required if kms-key is set)")
	diskCmd.Flags().String("kms-project", "", "KMS project ID (defaults to --project if not set)")
	diskCmd.Flags().String("region", "", "GCP region for regional disks (use either --zone or --region, not both)")
	diskCmd.Flags().String("zone", "", "GCP zone for zonal disks (use either --zone or --region, not both)")
	diskCmd.Flags().Bool("auto-approve", false, "Skip all interactive prompts and proceed with migration")
	diskCmd.Flags().Int("concurrency", 10, "Maximum number of concurrent disk migrations (1-200)")
	diskCmd.Flags().Bool("retain-name", true, "Reuse original disk name (deletes original). If false, creates new disk with suffix")
	diskCmd.Flags().Bool("dry-run", false, "Show what would be done without making any changes")

	if err := diskCmd.MarkFlagRequired("target-disk-type"); err != nil {
		logger.Errorf("Failed to mark target-disk-type as required: %s", err)
	}
}

func validateDiskCmdFlags(cmd *cobra.Command, args []string) error {
	zone, _ := cmd.Flags().GetString("zone")
	region, _ := cmd.Flags().GetString("region")
	if err := validation.ValidateLocationFlags(zone, region); err != nil {
		return err
	}

	kmsKey, _ := cmd.Flags().GetString("kms-key")
	kmsKeyRing, _ := cmd.Flags().GetString("kms-keyring")
	kmsLocation, _ := cmd.Flags().GetString("kms-location")
	if err := validation.ValidateKMSConfig(kmsKey, kmsKeyRing, kmsLocation); err != nil {
		return err
	}

	labelFilter, _ := cmd.Flags().GetString("label")
	if err := validation.ValidateLabelFilter(labelFilter); err != nil {
		return err
	}

	concurrency, _ := cmd.Flags().GetInt("concurrency")
	if err := validation.ValidateConcurrency(concurrency, 200); err != nil {
		return err
	}

	return nil
}

func runConvert(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	logger.Starting("Starting disk conversion process...")

	config, err := createMigrationConfig(cmd)
	if err != nil {
		return err
	}

	logger.Debugf("Configuration: %+v", config)
	ctx := context.Background()

	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close()

	discoveredDisks, err := migrator.DiscoverDisks(ctx, config, gcpClient)
	if err != nil {
		return err
	}
	if len(discoveredDisks) == 0 {
		logger.Info("No disks to migrate. Exiting.")
		return nil
	}

	migrationResults, err := migrator.MigrateDisks(ctx, config, gcpClient, discoveredDisks)
	if err != nil {
		logger.Errorf("Migration phase encountered errors: %v", err)
	}

	err = migrator.CleanupSnapshots(ctx, config, gcpClient, migrationResults)
	if err != nil {
		logger.Warnf("Cleanup phase encountered errors: %v", err)
	}

	migrator.GenerateReports(migrationResults)

	// Calculate and print completion summary
	summary := migrator.CalculateMigrationSummary(migrationResults, "disk")
	migrator.PrintCompletionSummary(summary, config, startTime)

	if summary.FailureCount > 0 {
		return fmt.Errorf("some migrations failed")
	}

	return nil
}
