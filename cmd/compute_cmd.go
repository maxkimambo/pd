package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/taskmanager"
	"github.com/maxkimambo/pd/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	gceTargetDiskType string
	gceLabelFilter    string
	gceKmsKey         string
	gceKmsKeyRing     string
	gceKmsLocation    string
	gceKmsProject     string
	gceRegion         string
	gceZone           string
	gceInstances      []string
	gceAutoApprove    bool
	gceMaxConcurrency int
	gceRetainName     bool
)

var computeCmd = &cobra.Command{
	Use:   "compute",
	Short: "Migrate attached persistent disks on GCE instances to a new disk type",
	Long: `Performs migration of persistent disks attached to specified GCE instances.

Identifies instances and their disks based on project, location (zone or region), and instance names.
For each targeted disk, it will (eventually):
1. Optionally stop the instance.
2. Detach the disk.
3. Create a snapshot (with optional KMS encryption).
4. Delete the original disk (if retaining name).
5. Recreate the disk from the snapshot with the target type.
6. Attach the new disk to the instance.
7. Optionally restart the instance.
8. Clean up snapshots afterwards.

Example:
pd migrate compute --project my-gcp-project --zone us-central1-a --instances vm1,vm2.. vm.N --target-disk-type hyperdisk-balanced
pd migrate compute --project my-gcp-project --region us-central1 --instances "*" --target-disk-type hyperdisk-balanced --auto-approve
`,
	PreRunE: validateComputeCmdFlags,
	RunE:    runGceConvert,
}

func init() {
	computeCmd.Flags().StringVarP(&projectID, "project", "p", "", "GCP Project ID (required)")
	computeCmd.Flags().StringVarP(&gceTargetDiskType, "target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, hyperdisk-balanced) (required)")
	computeCmd.Flags().StringVar(&gceLabelFilter, "label", "", "Label filter for disks in key=value format (optional)")
	computeCmd.Flags().StringVar(&gceKmsKey, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	computeCmd.Flags().StringVar(&gceKmsKeyRing, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceKmsLocation, "kms-location", "", "KMS Key location (required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceKmsProject, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceRegion, "region", "", "GCP region (required if zone is not set)")
	computeCmd.Flags().StringVar(&gceZone, "zone", "", "GCP zone (required if region is not set)")
	computeCmd.Flags().StringSliceVar(&gceInstances, "instances", nil, "Comma-separated list of instance names, or '*' for all instances in the scope (required)")
	computeCmd.Flags().BoolVar(&gceAutoApprove, "auto-approve", false, "Skip all interactive prompts")
	computeCmd.Flags().IntVar(&gceMaxConcurrency, "max-concurrency", 5, "Maximum number of disks/instances to process concurrently (1-50)")
	computeCmd.Flags().BoolVar(&gceRetainName, "retain-name", true, "Reuse original disk name. If false, keep original and suffix new name.")
	computeCmd.Flags().Int64Var(&throughput, "throughput", 140, "Throughput for the new disk in MiB/s (optional, default is 140)")
	computeCmd.Flags().Int64Var(&iops, "iops", 3000, "IOPS for the new disk (optional, default is 3000)")
	computeCmd.Flags().StringVarP(&storagePoolId, "pool-id", "s", "", "Storage pool ID to use for the new disks (optional)")
	computeCmd.MarkFlagRequired("target-disk-type")
	computeCmd.MarkFlagRequired("instances")
}

func validateComputeCmdFlags(cmd *cobra.Command, args []string) error {

	if projectID == "" { // projectID is from root persistent flag
		return errors.New("required flag --project not set")
	}

	if (gceZone == "" && gceRegion == "") || (gceZone != "" && gceRegion != "") {
		return errors.New("exactly one of --zone or --region must be specified")
	}

	if gceKmsKey != "" {
		if gceKmsKeyRing == "" || gceKmsLocation == "" {
			return errors.New("--kms-keyring and --kms-location are required when --kms-key is specified")
		}
		if gceKmsProject == "" {
			gceKmsProject = projectID
		}
	}

	if gceLabelFilter != "" && !strings.Contains(gceLabelFilter, "=") {
		return fmt.Errorf("invalid label format for --label: %s. Expected key=value", gceLabelFilter)
	}

	if gceMaxConcurrency < 1 || gceMaxConcurrency > 50 { // Adjusted max concurrency for instance operations
		return fmt.Errorf("--max-concurrency must be between 1 and 50, got %d", gceMaxConcurrency)
	}

	if len(gceInstances) == 0 {
		return errors.New("required flag --instances not set")
	}

	if throughput < 140 || throughput > 5000 {
		return fmt.Errorf("--throughput must be between 140 and 5000 MB/s, got %d", throughput)
	}

	if iops < 3000 || iops > 350000 {
		return fmt.Errorf("--iops must be between 3000 and 350,000, got %d", iops)
	}

	return nil
}

func runGceConvert(cmd *cobra.Command, args []string) error {
	logger.Starting("Starting disk migration process...")

	config := migrator.Config{
		ProjectID:      projectID,
		TargetDiskType: gceTargetDiskType,
		LabelFilter:    gceLabelFilter,
		KmsKey:         gceKmsKey,
		KmsKeyRing:     gceKmsKeyRing,
		KmsLocation:    gceKmsLocation,
		KmsProject:     gceKmsProject,
		Region:         gceRegion,
		Zone:           gceZone,
		AutoApproveAll: gceAutoApprove,
		Concurrency:    gceMaxConcurrency,
		RetainName:     gceRetainName,
		Instances:      gceInstances,
		Throughput:     throughput,
		Iops:           iops,
		StoragePoolId:  storagePoolId,
	}
	logger.Debugf("Configuration: %+v", config)
	logger.Infof("Project: %s", projectID)
	if gceZone != "" {
		logger.Infof("Zone: %s", gceZone)
	} else {
		logger.Infof("Region: %s", gceRegion)
	}
	if gceInstances[0] == "*" {
		logger.Infof("Target: All instances in scope (%s)", config.Location())
	} else {
		logger.Infof("Target: %s", strings.Join(gceInstances, ", "))
	}
	logger.Infof("Target disk type: %s", gceTargetDiskType)

	ctx := context.Background()
	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close()

	discoveredInstances, err := migrator.DiscoverInstances(ctx, &config, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to discover GCE instances: %w", err)
	}
	logger.Infof("Discovered %d instance(s) for migration.", len(discoveredInstances))

	if len(discoveredInstances) == 0 {
		return fmt.Errorf("no instances found matching the specified criteria")
	}

	// Create migration manager
	manager := workflow.NewMigrationManager(gcpClient, &config)

	logger.Starting("⚙️  Migration Phase: GCE Attached Disks")

	// Track overall results
	var successCount, failureCount int
	var allResults []*workflow.WorkflowResult
	workflows := make([]*taskmanager.Workflow, 0, len(discoveredInstances))
	for _, instance := range discoveredInstances {
		logger.Infof("Processing instance: %s", *instance.Name)
		// Create instance-specific workflow
		workflow, err := manager.CreateInstanceMigrationWorkflow(instance)
		if err != nil {
			logger.Errorf("Failed to create workflow for %s: %v", *instance.Name, err)
			failureCount++
			continue
		}
		workflows = append(workflows, workflow)
	}

	// Execute prepared workflows
	for _, workflow := range workflows {

		// Execute workflow
		result, err := manager.ExecuteWorkflow(ctx, workflow)
		allResults = append(allResults, result)

		if err != nil {
			logger.Errorf("Workflow failed for %s: %v", workflow.ID, err)
			failureCount++
		} else {
			successCount++
		}
	}

	// Summary Report
	logger.Info("\n🎯 Migration Summary\n" + strings.Repeat("=", 25))
	logger.Infof("Total instances processed: %d", len(discoveredInstances))
	logger.Infof("Successful migrations: %d", successCount)
	logger.Infof("Failed migrations: %d", failureCount)

	for _, result := range allResults {
		// Report individual results
		manager.ReportResults(result)
	}

	if failureCount > 0 {
		return fmt.Errorf("%d out of %d instance migrations failed", failureCount, len(discoveredInstances))
	}

	logger.Success("All instance migrations completed successfully!")
	return nil
}
