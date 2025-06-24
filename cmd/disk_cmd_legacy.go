package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/spf13/cobra"
)

var (
	targetDiskTypeLegacy string
	labelFilterLegacy    string
	kmsKeyLegacy         string
	kmsKeyRingLegacy     string
	kmsLocationLegacy    string
	kmsProjectLegacy     string
	regionLegacy         string
	zoneLegacy           string
	autoApproveLegacy    bool
	concurrencyLegacy    int
	retainNameLegacy     bool
	throughputLegacy     int64
	iopsLegacy           int64
	storagePoolIdLegacy  string
)

var diskCmdLegacy = &cobra.Command{
	Use:   "disk",
	Short: "Migrate detached persistent disks to a new disk type",
	Long: `Performs bulk migration of detached Google Cloud persistent disks.

Identifies detached disks based on location (zone or region) and optional label filter.
Migrates disks by:
1. Creating a snapshot (with optional KMS encryption).
2. Deleting the original disk (optional, controlled by --retain-name).
3. Recreating the disk from the snapshot with the target type.
4. Cleaning up snapshots afterwards.

Example:
pd migrate disk --project my-gcp-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging --max-concurrency 20
pd migrate disk --project my-gcp-project --region us-central1 --target-disk-type hyperdisk-balanced --auto-approve --retain-name=false
`,
	PreRunE: validateDiskCmdFlagsLegacy,
	RunE:    runConvertLegacy,
}

func init() {
	diskCmdLegacy.Flags().StringVarP(&targetDiskTypeLegacy, "target-disk-type", "t", "", "Target disk type (e.g. pd-ssd, hyperdisk-balanced) (required)")
	diskCmdLegacy.Flags().StringVar(&labelFilterLegacy, "label", "", "Label filter in key=value format (optional)")
	diskCmdLegacy.Flags().StringVar(&kmsKeyLegacy, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	diskCmdLegacy.Flags().StringVar(&kmsKeyRingLegacy, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	diskCmdLegacy.Flags().StringVar(&kmsLocationLegacy, "kms-location", "", "KMS Key location (required if kms-key is set)")
	diskCmdLegacy.Flags().StringVar(&kmsProjectLegacy, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	diskCmdLegacy.Flags().StringVar(&regionLegacy, "region", "", "GCP region (required if zone is not set)")
	diskCmdLegacy.Flags().StringVar(&zoneLegacy, "zone", "", "GCP zone (required if region is not set)")
	diskCmdLegacy.Flags().BoolVar(&autoApproveLegacy, "auto-approve", true, "Skip all interactive prompts")
	diskCmdLegacy.Flags().IntVar(&concurrencyLegacy, "concurrency", 10, "Number of disks to process concurrently (1-200), default: 10")
	diskCmdLegacy.Flags().BoolVar(&retainNameLegacy, "retain-name", true, "Reuse original disk name (delete original). If false, keep original and suffix new name.")
	diskCmdLegacy.Flags().Int64Var(&throughputLegacy, "throughput", 140, "Throughput in MB/s to set (optional, default: 140 MiB/s)")
	diskCmdLegacy.Flags().Int64Var(&iopsLegacy, "iops", 2000, "IOPS to set(optional, default: 2000 IOPS)")
	diskCmdLegacy.Flags().StringVarP(&storagePoolIdLegacy, "pool-id", "s", "", "Storage pool ID to use for the new disks (optional)")
	_ = diskCmdLegacy.MarkFlagRequired("target-disk-type")
}

func validateDiskCmdFlagsLegacy(cmd *cobra.Command, args []string) error {
	if projectID == "" {
		return errors.New("required flag --project not set")
	}

	if (zoneLegacy == "" && regionLegacy == "") || (zoneLegacy != "" && regionLegacy != "") {
		return errors.New("exactly one of --zone or --region must be specified")
	}

	if kmsKeyLegacy != "" {
		if kmsKeyRingLegacy == "" || kmsLocationLegacy == "" {
			return errors.New("--kms-keyring and --kms-location are required when --kms-key is specified")
		}
		if kmsProjectLegacy == "" {
			kmsProjectLegacy = projectID
		}
	}

	if labelFilterLegacy != "" && !strings.Contains(labelFilterLegacy, "=") {
		return fmt.Errorf("invalid label format: %s. Expected key=value", labelFilterLegacy)
	}

	if concurrencyLegacy < 1 || concurrencyLegacy > 200 {
		return fmt.Errorf("--max-concurrency must be between 1 and 200, got %d", concurrencyLegacy)
	}
	if throughputLegacy < 140 || throughputLegacy > 5000 {
		return fmt.Errorf("--throughput must be between 0 and 5000 MB/s, got %d", throughputLegacy)
	}

	if iopsLegacy < 2000 || iopsLegacy > 350000 {
		return fmt.Errorf("--iops must be between 3000 and 350,000, got %d", iopsLegacy)
	}

	return nil
}

func runConvertLegacy(cmd *cobra.Command, args []string) error {
	// Set verbose to true if debug is enabled for backward compatibility
	if debug {
		verbose = true
	}
	logger.Setup(verbose, jsonLogs, quiet)

	logger.User.Starting("Starting disk conversion process...")

	config := migrator.Config{
		ProjectID:      projectID,
		TargetDiskType: targetDiskTypeLegacy,
		LabelFilter:    labelFilterLegacy,
		KmsKey:         kmsKeyLegacy,
		KmsKeyRing:     kmsKeyRingLegacy,
		KmsLocation:    kmsLocationLegacy,
		KmsProject:     kmsProjectLegacy,
		Region:         regionLegacy,
		Zone:           zoneLegacy,
		AutoApproveAll: autoApproveLegacy,
		Concurrency:    concurrencyLegacy,
		RetainName:     retainNameLegacy,
		Debug:          debug,
		Throughput:     throughputLegacy,
		Iops:           iopsLegacy,
	}
	logger.Op.Debugf("Configuration: %+v", config)
	ctx := context.Background()

	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close()

	discoveredDisks, err := migrator.DiscoverDisks(ctx, &config, gcpClient)
	if err != nil {
		return err
	}
	if len(discoveredDisks) == 0 {
		logger.User.Info("No disks to migrate. Exiting.")
		return nil
	}

	migrationResults, err := migrator.MigrateDisks(ctx, &config, gcpClient, discoveredDisks)
	if err != nil {
		logger.User.Errorf("Migration phase encountered errors: %v", err)
	}

	err = migrator.CleanupSnapshots(ctx, &config, gcpClient, migrationResults)
	if err != nil {
		logger.User.Warnf("Cleanup phase encountered errors: %v", err)
	}

	migrator.GenerateReports(migrationResults)

	logger.User.Success("Disk conversion process finished.")
	return nil
}
