package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/orchestrator"
	"github.com/maxkimambo/pd/internal/utils"
	"github.com/maxkimambo/pd/internal/validation"
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
	gceThroughput     int64
	gceIops           int64
)

var computeCmd = &cobra.Command{
	Use:   "compute",
	Short: "Migrate attached persistent disks on GCE instances to a new disk type",
	Long: `Performs migration of persistent disks attached to specified GCE instances using task orchestration.

Identifies instances and their disks based on project, location (zone or region), and instance names.
For each targeted disk, it will:
1. Optionally stop the instance.
2. Detach the disk.
3. Create a snapshot (with optional KMS encryption).
4. Delete the original disk (if retaining name).
5. Recreate the disk from the snapshot with the target type.
6. Attach the new disk to the instance.
7. Optionally restart the instance.
8. Clean up snapshots afterwards.

The orchestration provides improved parallelism and dependency management with detailed progress logging.

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
	computeCmd.Flags().Int64Var(&gceThroughput, "throughput", 150, "Throughput for the new disk in MiB/s (optional, default is 150)")
	computeCmd.Flags().Int64Var(&gceIops, "iops", 3000, "IOPS for the new disk (optional, default is 3000)")

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

	return nil
}

func runGceConvert(cmd *cobra.Command, args []string) error {
	// Set verbose to true if debug is enabled for backward compatibility
	if debug {
		verbose = true
	}
	logger.Setup(verbose, jsonLogs, quiet)

	logger.User.Starting("Starting disk migration process...")

	// trim leading/trailing whitespace from instance names
	for i, instance := range gceInstances {
		gceInstances[i] = strings.TrimSpace(instance)
	}

	config := migrator.Config{
		ProjectID:        projectID,
		TargetDiskType:   gceTargetDiskType,
		LabelFilter:      gceLabelFilter,
		KmsKey:           gceKmsKey,
		KmsKeyRing:       gceKmsKeyRing,
		KmsLocation:      gceKmsLocation,
		KmsProject:       gceKmsProject,
		Region:           gceRegion,
		Zone:             gceZone,
		AutoApproveAll:   gceAutoApprove,
		Concurrency:      gceMaxConcurrency,
		MaxParallelTasks: gceMaxConcurrency, // Map concurrency to task parallelism
		RetainName:       gceRetainName,
		Debug:            debug,
		Instances:        gceInstances,
		Throughput:       gceThroughput,
		Iops:             gceIops,
	}

	logger.Op.Debugf("Configuration: %+v", config)
	logger.User.Infof("Project: %s", projectID)
	if gceZone != "" {
		logger.User.Infof("Zone: %s", gceZone)
	} else {
		logger.User.Infof("Region: %s", gceRegion)
	}
	if gceInstances[0] == "*" {
		logger.User.Infof("Target: All instances in scope (%s)", config.Location())
	} else {
		logger.User.Infof("Target: %s", strings.Join(gceInstances, ", "))
	}
	logger.User.Infof("Target disk type: %s", gceTargetDiskType)

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
	logger.User.Infof("Discovered %d instance(s) eligible for migration.", len(discoveredInstances))

	// Validate instances against target disk type
	validatedInstances, failedInstances, err := validateInstancesForMigration(ctx, discoveredInstances, gceTargetDiskType, projectID, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to validate instances for migration: %w", err)
	}

	// Report validation results
	if len(failedInstances) > 0 {
		logger.User.Warnf("--- Validation Failed for %d Instance(s) ---", len(failedInstances))
		for _, failedInstance := range failedInstances {
			logger.User.Warnf("  ❌ %s: %s", failedInstance.InstanceName, failedInstance.Reason)
		}
		logger.User.Info("--- End Validation Failures ---")
	}

	if len(validatedInstances) == 0 {
		return fmt.Errorf("no instances passed validation for target disk type %s", gceTargetDiskType)
	}

	logger.User.Infof("✅ %d instance(s) validated successfully for migration.", len(validatedInstances))

	// Create task orchestrator
	taskOrchestrator := orchestrator.NewDAGOrchestrator(&config, gcpClient)

	// Execute migration workflow with validated instances
	result, err := taskOrchestrator.ExecuteInstanceMigrations(ctx, validatedInstances)
	if err != nil {
		return fmt.Errorf("migration workflow failed: %w", err)
	}

	// --- Phase 3: Results Summary ---
	logger.User.Info("--- Phase 3: Results Summary ---")
	err = taskOrchestrator.ProcessExecutionResults(result)
	if err != nil {
		return fmt.Errorf("migration completed with errors: %w", err)
	}

	logger.User.Success("Task-based disk migration process completed successfully.")
	return nil
}

// ValidationFailure represents an instance that failed validation
type ValidationFailure struct {
	InstanceName string
	Reason       string
}

// validateInstancesForMigration validates that all instances can support the target disk type
func validateInstancesForMigration(ctx context.Context, instances []*computepb.Instance, targetDiskType string, projectID string, gcpClient *gcp.Clients) ([]*computepb.Instance, []ValidationFailure, error) {
	var validatedInstances []*computepb.Instance
	var failedInstances []ValidationFailure

	logger.User.Info("--- Validating instance compatibility before migration ---")
	logger.User.Infof("Validating %d instance(s) against target disk type: %s", len(instances), targetDiskType)

	for _, instance := range instances {
		instanceName := instance.GetName()
		machineType := utils.ExtractMachineType(instance.GetMachineType())

		// Validate machine type against target disk type
		validationResult := validation.IsCompatible(machineType, targetDiskType)
		logger.User.Infof("Instance %s: Machine type %s validation against target disk type %s: %v", instanceName, machineType, targetDiskType, validationResult)
		if !validationResult.Compatible {
			logger.User.Warnf("Instance %s validation failed: %s", instanceName, validationResult.Reason)
			failedInstances = append(failedInstances, ValidationFailure{
				InstanceName: instanceName,
				Reason:       validationResult.Reason,
			})
			logger.Op.Debugf("❌ Instance %s failed validation: %s", instanceName, validationResult.Reason)
			continue
		}

		// Validate all attached disks can be migrated to target type
		if instance.Disks != nil {
			diskValidationPassed := true
			var diskFailureReasons []string

			for _, attachedDisk := range instance.Disks {
				if attachedDisk.GetSource() == "" {
					continue // Skip disks without source (e.g., local-ssd)
				}

				// Get disk name from source URL
				diskName := ""
				if attachedDisk.GetSource() != "" {
					parts := strings.Split(attachedDisk.GetSource(), "/")
					diskName = parts[len(parts)-1]
				}

				if diskName == "" {
					continue
				}

				// Get zone name from instance zone
				zone := utils.ExtractZoneName(instance.GetZone())

				disk, err := gcpClient.DiskClient.GetDisk(ctx, projectID, zone, diskName)
				if err != nil {
					logger.Op.Debugf("Warning: Could not get disk details for %s: %v", diskName, err)
					continue
				}

				// Check if disk is already the target type
				currentDiskType := utils.ExtractDiskType(disk.GetType())

				if currentDiskType == targetDiskType {
					logger.Op.Debugf("Disk %s already has target type %s, skipping", diskName, targetDiskType)
					continue
				}

				// Validate current disk type can be migrated to target type
				// For now, we'll allow all migrations except those that are clearly incompatible
				if currentDiskType == "local-ssd" {
					diskValidationPassed = false
					diskFailureReasons = append(diskFailureReasons, fmt.Sprintf("disk %s has type local-ssd which cannot be migrated", diskName))
				}
			}

			if !diskValidationPassed {
				reason := fmt.Sprintf("Disk validation failed: %s", strings.Join(diskFailureReasons, "; "))
				failedInstances = append(failedInstances, ValidationFailure{
					InstanceName: instanceName,
					Reason:       reason,
				})
				logger.Op.Debugf("❌ Instance %s failed disk validation: %s", instanceName, reason)
				continue
			}
		}

		// Instance passed all validation checks
		validatedInstances = append(validatedInstances, instance)
		logger.Op.Debugf("✅ Instance %s passed validation", instanceName)
	}

	logger.User.Infof("Validation complete: %d passed, %d failed", len(validatedInstances), len(failedInstances))
	return validatedInstances, failedInstances, nil
}
