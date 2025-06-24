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
	Short: "Migrate detached persistent disks to a new disk type",
	Long: `Performs bulk migration of detached Google Cloud persistent disks using task orchestration.

Identifies detached disks based on location (zone or region) and optional label filter.
Migrates disks by:
1. Creating a snapshot (with optional KMS encryption).
2. Deleting the original disk (optional, controlled by --retain-name).
3. Recreating the disk from the snapshot with the target type.
4. Cleaning up snapshots afterwards.

The orchestration provides improved parallelism and dependency management with detailed progress logging.

Example:
pd migrate disk --project my-gcp-project --zone us-central1-a --target-disk-type pd-ssd --label env=staging --max-concurrency 20
pd migrate disk --project my-gcp-project --region us-central1 --target-disk-type hyperdisk-balanced --auto-approve --retain-name=false
`,
	PreRunE: validateDiskCmdFlags,
	RunE:    runConvert,
}

func init() {
	diskCmd.Flags().StringVarP(&targetDiskType, "target-disk-type", "t", "", "Target disk type (e.g. pd-ssd, hyperdisk-balanced) (required)")
	diskCmd.Flags().StringVar(&labelFilter, "label", "", "Label filter in key=value format (optional)")
	diskCmd.Flags().StringVar(&kmsKey, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	diskCmd.Flags().StringVar(&kmsKeyRing, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	diskCmd.Flags().StringVar(&kmsLocation, "kms-location", "", "KMS Key location (required if kms-key is set)")
	diskCmd.Flags().StringVar(&kmsProject, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	diskCmd.Flags().StringVar(&region, "region", "", "GCP region (required if zone is not set)")
	diskCmd.Flags().StringVar(&zone, "zone", "", "GCP zone (required if region is not set)")
	diskCmd.Flags().BoolVar(&autoApprove, "auto-approve", true, "Skip all interactive prompts")
	diskCmd.Flags().IntVar(&concurrency, "concurrency", 10, "Number of disks to process concurrently (1-200), default: 10")
	diskCmd.Flags().BoolVar(&retainName, "retain-name", true, "Reuse original disk name (delete original). If false, keep original and suffix new name.")
	diskCmd.Flags().Int64Var(&throughput, "throughput", 140, "Throughput in MB/s to set (optional, default: 140 MiB/s)")
	diskCmd.Flags().Int64Var(&iops, "iops", 2000, "IOPS to set(optional, default: 2000 IOPS)")
	diskCmd.Flags().StringVarP(&storagePoolId, "pool-id", "s", "", "Storage pool ID to use for the new disks (optional)")
	
	diskCmd.MarkFlagRequired("target-disk-type")
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
		return fmt.Errorf("--max-concurrency must be between 1 and 200, got %d", concurrency)
	}
	if throughput < 140 || throughput > 5000 {
		return fmt.Errorf("--throughput must be between 0 and 5000 MB/s, got %d", throughput)
	}

	if iops < 2000 || iops > 350000 {
		return fmt.Errorf("--iops must be between 3000 and 350,000, got %d", iops)
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

	// --- Phase 1: Disk Discovery ---
	logger.User.Info("--- Phase 1: Disk Discovery ---")
	discoveredDisks, err := migrator.DiscoverDisks(ctx, &config, gcpClient)
	if err != nil {
		return err
	}
	if len(discoveredDisks) == 0 {
		logger.User.Info("No disks to migrate. Exiting.")
		return nil
	}
	logger.User.Infof("Discovered %d disk(s) for migration.", len(discoveredDisks))

	// --- Phase 2: Task-based Migration ---
	logger.User.Info("--- Phase 2: Task-based Migration (Detached Disks) ---")

	// Create task orchestrator
	taskOrchestrator := orchestrator.NewDAGOrchestrator(&config, gcpClient)

	// Execute migration workflow
	result, err := taskOrchestrator.ExecuteDiskMigrations(ctx, discoveredDisks)
	if err != nil {
		return fmt.Errorf("migration workflow failed: %w", err)
	}

	// --- Phase 3: Results Summary ---
	logger.User.Info("--- Phase 3: Results Summary ---")
	err = taskOrchestrator.ProcessExecutionResults(result)
	if err != nil {
		return fmt.Errorf("migration completed with errors: %w", err)
	}

	logger.User.Success("Task-based disk conversion process finished.")
	return nil
}