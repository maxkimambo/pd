package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/orchestrator"
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
	autoApprove    bool
	concurrency    int
	retainName     bool
	throughput     int64
	iops           int64
	storagePoolId  string
)

var diskCmd = &cobra.Command{
	Use:   "disk",
	Short: "Migrate detached persistent disks to new disk types",
	Long: `Migrate detached Google Cloud persistent disks to new disk types using task orchestration.

WHAT ARE DETACHED DISKS?
Detached disks are persistent disks that are not currently attached to any Compute Engine 
instance. These can be migrated safely without affecting running workloads.

MIGRATION PROCESS:
1. Discovery: Find detached disks matching location and label criteria
2. Validation: Verify disks can be migrated to target type
3. Snapshot: Create snapshots with optional KMS encryption
4. Recreation: Create new disks from snapshots with target specifications
5. Cleanup: Remove intermediate snapshots automatically
6. Reporting: Provide detailed migration results

IMPORTANT CONSIDERATIONS:
⚠️  Backup Recommendation: Ensure additional backups exist before migration
⚠️  Quota Requirements: Temporary snapshots require additional storage quota
⚠️  Cost Impact: Snapshots incur storage costs during migration window
⚠️  Disk Names: --retain-name=true deletes original disks (irreversible)

COMMON USE CASES:
• Upgrade pd-standard to pd-ssd for better performance
• Migrate to hyperdisk-balanced for improved IOPS and throughput
• Consolidate disks to newer storage pool technologies
• Optimize costs by moving to appropriate disk types

EXAMPLES:
# Migrate staging disks in a specific zone
pd migrate disk --project my-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging

# Bulk migrate all detached disks in a region (interactive confirmation)
pd migrate disk --project my-project --region us-central1 --target-disk-type hyperdisk-balanced

# High-throughput migration with custom settings
pd migrate disk --project my-project --zone us-west1-b --target-disk-type hyperdisk-throughput --throughput 2000 --concurrency 15

# Safe migration keeping original disks
pd migrate disk --project my-project --zone europe-west1-c --target-disk-type pd-ssd --retain-name=false --auto-approve=false
`,
	PreRunE: validateDiskCmdFlags,
	RunE:    runConvert,
}

func init() {
	diskCmd.Flags().StringVarP(&targetDiskType, "target-disk-type", "t", "", "Target disk type (pd-ssd, pd-standard, hyperdisk-balanced, etc.) (required)")
	diskCmd.Flags().StringVar(&labelFilter, "label", "", "Filter disks by label in key=value format (e.g., env=prod)")
	diskCmd.Flags().StringVar(&kmsKey, "kms-key", "", "KMS key name for snapshot encryption (enhances security)")
	diskCmd.Flags().StringVar(&kmsKeyRing, "kms-keyring", "", "KMS key ring name (required when using --kms-key)")
	diskCmd.Flags().StringVar(&kmsLocation, "kms-location", "", "KMS key location/region (required when using --kms-key)")
	diskCmd.Flags().StringVar(&kmsProject, "kms-project", "", "KMS project ID (defaults to --project if not specified)")
	diskCmd.Flags().StringVar(&region, "region", "", "GCP region to search for disks (mutually exclusive with --zone)")
	diskCmd.Flags().StringVar(&zone, "zone", "", "GCP zone to search for disks (mutually exclusive with --region)")
	diskCmd.Flags().BoolVar(&autoApprove, "auto-approve", true, "Skip interactive confirmations (default: true for automation)")
	diskCmd.Flags().IntVar(&concurrency, "concurrency", 10, "Number of disks to process simultaneously (1-200, default: 10)")
	diskCmd.Flags().BoolVar(&retainName, "retain-name", true, "Reuse original disk name by deleting original (default: true, irreversible)")
	diskCmd.Flags().Int64Var(&throughput, "throughput", 140, "Disk throughput in MiB/s (applicable to hyperdisk types, default: 140)")
	diskCmd.Flags().Int64Var(&iops, "iops", 2000, "Disk IOPS limit (applicable to hyperdisk types, default: 2000)")
	diskCmd.Flags().StringVarP(&storagePoolId, "pool-id", "s", "", "Storage pool ID for new disks (advanced feature for storage pools)")

	_ = diskCmd.MarkFlagRequired("target-disk-type")
}

func validateDiskCmdFlags(cmd *cobra.Command, args []string) error {
	if projectID == "" {
		return errors.New("required flag --project not set")
	}

	if (zone == "" && region == "") || (zone != "" && region != "") {
		return errors.New("exactly one of --zone or --region must be specified")
	}

	if kmsKey != "" {
		if kmsKeyRing == "" || kmsLocation == "" {
			return errors.New("--kms-keyring and --kms-location are required when --kms-key is specified")
		}
		if kmsProject == "" {
			kmsProject = projectID
		}
	}

	if labelFilter != "" && !strings.Contains(labelFilter, "=") {
		return fmt.Errorf("invalid label format: %s. Expected key=value", labelFilter)
	}

	if concurrency < 1 || concurrency > 200 {
		return fmt.Errorf("--concurrency must be between 1 and 200, got %d", concurrency)
	}
	if throughput < 140 || throughput > 5000 {
		return fmt.Errorf("--throughput must be between 140 and 5000 MiB/s, got %d", throughput)
	}

	if iops < 2000 || iops > 350000 {
		return fmt.Errorf("--iops must be between 2000 and 350,000, got %d", iops)
	}

	return nil
}

func runConvert(cmd *cobra.Command, args []string) error {
	// Set verbose to true if debug is enabled for backward compatibility
	if debug {
		verbose = true
	}
	logger.Setup(verbose, jsonLogs, quiet)

	logger.User.Starting("Starting task-based disk conversion process...")

	config := migrator.Config{
		ProjectID:        projectID,
		TargetDiskType:   targetDiskType,
		LabelFilter:      labelFilter,
		KmsKey:           kmsKey,
		KmsKeyRing:       kmsKeyRing,
		KmsLocation:      kmsLocation,
		KmsProject:       kmsProject,
		Region:           region,
		Zone:             zone,
		AutoApproveAll:   autoApprove,
		Concurrency:      concurrency,
		MaxParallelTasks: concurrency, // Map concurrency to task parallelism
		RetainName:       retainName,
		Debug:            debug,
		Throughput:       throughput,
		Iops:             iops,
	}

	logger.Op.Debugf("Configuration: %+v", config)
	logger.User.Infof("Project: %s", projectID)
	if zone != "" {
		logger.User.Infof("Zone: %s", zone)
	} else {
		logger.User.Infof("Region: %s", region)
	}
	logger.User.Infof("Target disk type: %s", targetDiskType)

	ctx := context.Background()

	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close()

	logger.User.Info("=== DISCOVERY ===")
	discoveredDisks, err := migrator.DiscoverDisks(ctx, &config, gcpClient)
	if err != nil {
		return err
	}
	if len(discoveredDisks) == 0 {
		logger.User.Info("No disks to migrate. Exiting.")
		return nil
	}
	logger.User.Infof("Discovered %d disk(s) for migration.", len(discoveredDisks))

	logger.User.Info("=== MIGRATION ===")

	// Create task orchestrator
	taskOrchestrator := orchestrator.NewDAGOrchestrator(&config, gcpClient)

	// Execute migration workflow
	result, err := taskOrchestrator.ExecuteDiskMigrations(ctx, discoveredDisks)
	if err != nil {
		return fmt.Errorf("migration workflow failed: %w", err)
	}

	logger.User.Info("=== RESULTS ===")
	err = taskOrchestrator.ProcessExecutionResults(result)
	if err != nil {
		return fmt.Errorf("migration completed with errors: %w", err)
	}

	logger.User.Success("Task-based disk conversion process finished.")
	return nil
}
