package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gcp-disk-migrator/internal/gcp"
	"gcp-disk-migrator/internal/logger"
	"gcp-disk-migrator/internal/migrator" // Placeholder for actual migrator package

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	targetDiskType string
	labelFilter    string
	kmsKey         string
	kmsKeyRing     string
	kmsLocation    string
	kmsProject     string
	region         string
	zone           string
	yes            bool
	autoApprove    bool
	maxConcurrency int
	retainName     bool
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Migrate detached persistent disks to a new type",
	Long: `Performs bulk migration of detached Google Cloud Persistent Disks.

Identifies detached disks based on location (zone or region) and optional label filter.
Migrates disks by:
1. Creating a snapshot (with optional KMS encryption).
2. Deleting the original disk (optional, controlled by --retain-name).
3. Recreating the disk from the snapshot with the target type.
4. Cleaning up snapshots afterwards.

Example:
pd convert --project my-gcp-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging --max-concurrency 20
pd convert --project my-gcp-project --region us-central1 --target-disk-type hyperdisk-balanced --auto-approve --retain-name=false
`,
	PreRunE: validateConvertFlags, // Use PreRunE for validation before RunE
	RunE:    runConvert,
}

func init() {
	rootCmd.AddCommand(convertCmd)

	// Define flags for the convert command
	convertCmd.Flags().StringVarP(&targetDiskType, "target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, hyperdisk-balanced) (required)")
	convertCmd.Flags().StringVar(&labelFilter, "label", "", "Label filter in key=value format (optional)")
	convertCmd.Flags().StringVar(&kmsKey, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	convertCmd.Flags().StringVar(&kmsKeyRing, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	convertCmd.Flags().StringVar(&kmsLocation, "kms-location", "", "KMS Key location (required if kms-key is set)")
	convertCmd.Flags().StringVar(&kmsProject, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	convertCmd.Flags().StringVar(&region, "region", "", "GCP region (required if zone is not set)")
	convertCmd.Flags().StringVar(&zone, "zone", "", "GCP zone (required if region is not set)")
	convertCmd.Flags().BoolVar(&yes, "yes", false, "Skip initial disk list confirmation")
	convertCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip all interactive prompts (overrides --yes)")
	convertCmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 10, "Maximum number of disks to process concurrently (1-200)")
	convertCmd.Flags().BoolVar(&retainName, "retain-name", true, "Reuse original disk name (delete original). If false, keep original and suffix new name.")

	// Mark required flags
	convertCmd.MarkFlagRequired("target-disk-type")
	// Note: project, zone/region are validated in PreRunE
}

// validateConvertFlags checks the validity of flags before running the command.
func validateConvertFlags(cmd *cobra.Command, args []string) error {
	// Project ID check (inherited from root, but check explicitly here)
	if projectID == "" {
		return errors.New("required flag --project not set")
	}

	// Zone vs Region check
	if (zone == "" && region == "") || (zone != "" && region != "") {
		return errors.New("exactly one of --zone or --region must be specified")
	}

	// KMS flags validation
	if kmsKey != "" {
		if kmsKeyRing == "" || kmsLocation == "" {
			return errors.New("--kms-keyring and --kms-location are required when --kms-key is specified")
		}
		// Default kmsProject to projectID if not set
		if kmsProject == "" {
			kmsProject = projectID
		}
	}

	// Label filter format check (basic)
	if labelFilter != "" && !strings.Contains(labelFilter, "=") {
		return fmt.Errorf("invalid label format: %s. Expected key=value", labelFilter)
	}

	// Concurrency range check
	if maxConcurrency < 1 || maxConcurrency > 200 {
		return fmt.Errorf("--max-concurrency must be between 1 and 200, got %d", maxConcurrency)
	}

	return nil
}

// runConvert executes the main logic for the conversion.
func runConvert(cmd *cobra.Command, args []string) error {
	// Setup logger first using the global debug flag
	logger.Setup(debug)

	logrus.Info("Starting disk conversion process...")

	// Prepare configuration for the migrator
	config := migrator.Config{
		ProjectID:      projectID,
		TargetDiskType: targetDiskType,
		LabelFilter:    labelFilter,
		KmsKey:         kmsKey,
		KmsKeyRing:     kmsKeyRing,
		KmsLocation:    kmsLocation,
		KmsProject:     kmsProject,
		Region:         region,
		Zone:           zone,
		SkipConfirm:    yes || autoApprove,
		AutoApproveAll: autoApprove,
		MaxConcurrency: maxConcurrency,
		RetainName:     retainName,
		Debug:          debug,
	}

	logrus.WithField("config", fmt.Sprintf("%+v", config)).Debug("Migration configuration prepared")

	// Initialize GCP Client
	ctx := context.Background()
	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		// Logrus already logs errors, just return it
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close() // Ensure clients are closed on exit

	// --- Execute Phases ---

	// Phase 1: Discovery
	discoveredDisks, err := migrator.DiscoverDisks(ctx, &config, gcpClient)
	if err != nil {
		// Error already logged by DiscoverDisks
		return err // Exit if discovery fails critically
	}
	if len(discoveredDisks) == 0 {
		logrus.Info("No disks to migrate. Exiting.")
		return nil // Successful exit, nothing to do
	}

	// Phase 2: Migration
	migrationResults, err := migrator.MigrateDisks(ctx, &config, gcpClient, discoveredDisks)
	if err != nil {
		// Error should be logged within MigrateDisks or migrateSingleDisk
		// We might still proceed to cleanup and reporting even if migration has errors
		logrus.Errorf("Migration phase encountered errors: %v", err)
		// Decide if we should exit or continue to cleanup/report
		// For now, let's continue to allow cleanup and reporting of partial success/failures
	}

	// Phase 3: Cleanup
	// Pass migrationResults so cleanup can update SnapshotCleaned status
	err = migrator.CleanupSnapshots(ctx, &config, gcpClient, migrationResults)
	if err != nil {
		// Error logged by CleanupSnapshots
		logrus.Warnf("Cleanup phase encountered errors: %v", err)
		// Don't exit, proceed to reporting
	}

	// Phase 4: Reporting
	migrator.GenerateReports(migrationResults)

	logrus.Info("Disk conversion process finished.")
	return nil // Return nil on success, or an error if the process fails critically
}
