package cmd

import (
	"context"
	"fmt"
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/orchestrator"
	"github.com/maxkimambo/pd/internal/utils"
	"github.com/maxkimambo/pd/internal/validation"
	migerrors "github.com/maxkimambo/pd/internal/errors"
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
	Short: "Migrate persistent disks attached to Compute Engine instances",
	Long: `Migrate persistent disks attached to Compute Engine instances to new disk types with minimal downtime.

WHAT THIS COMMAND DOES:
Migrates non-boot persistent disks attached to Compute Engine instances by orchestrating
a series of operations designed to minimize service interruption.

MIGRATION PROCESS:
1. Discovery: Find instances and validate their attached disks
2. Compatibility Check: Verify machine types support target disk type
3. Instance Management: Stop instances if required for disk operations
4. Disk Operations: Detach → Snapshot → Recreate → Reattach disks
5. Instance Restart: Start instances after successful disk migration
6. Cleanup: Remove intermediate snapshots automatically

⚠️  IMPORTANT LIMITATIONS & WARNINGS:
• BOOT DISKS ARE NOT MIGRATED (requires special procedures)
• Instance downtime is required for disk detachment/reattachment
• All attached disks on target instances will be processed
• Machine type must be compatible with target disk type
• Regional persistent disks are NOT SUPPORTED

INSTANCE TARGETING:
• Specific instances: --instances vm1,vm2,vm3
• All instances in scope: --instances "*" (use quotes)
• Zone-based: Targets instances in specified zone only
• Region-based: Targets instances across all zones in region

DOWNTIME CONSIDERATIONS:
• Instances are stopped during disk operations (typically 5-15 minutes)
• Applications should be gracefully shut down before migration
• Consider maintenance windows for production workloads
• Persistent disk data is preserved throughout the process

COMPATIBILITY REQUIREMENTS:
• Machine types must support target disk type (validated automatically)
• Sufficient quota for snapshots and new disks
• No conflicting disk names in target project/zone
• Instance service accounts need appropriate permissions

EXAMPLES:
# Migrate specific instances in a zone
pd migrate compute --project my-project --zone us-central1-a --instances "web-server-1,web-server-2" --target-disk-type pd-ssd

# Migrate all instances in a region (with confirmation)
pd migrate compute --project my-project --region us-central1 --instances "*" --target-disk-type hyperdisk-balanced

# Production migration with custom settings
pd migrate compute --project prod-project --zone us-east1-b --instances "app-cluster-*" --target-disk-type hyperdisk-throughput --throughput 500 --max-concurrency 3

# Safe migration keeping original disks (testing)
pd migrate compute --project test-project --zone europe-west1-c --instances "test-vm" --target-disk-type pd-ssd --retain-name=false --auto-approve=false
`,
	PreRunE: validateComputeCmdFlags,
	RunE:    runGceConvert,
}

func init() {
	computeCmd.Flags().StringVarP(&projectID, "project", "p", "", "GCP Project ID where instances are located (required)")
	computeCmd.Flags().StringVarP(&gceTargetDiskType, "target-disk-type", "t", "", "Target disk type (pd-ssd, hyperdisk-balanced, etc.) (required)")
	computeCmd.Flags().StringVar(&gceLabelFilter, "label", "", "Filter instance disks by label in key=value format")
	computeCmd.Flags().StringVar(&gceKmsKey, "kms-key", "", "KMS key name for snapshot encryption (enhances security)")
	computeCmd.Flags().StringVar(&gceKmsKeyRing, "kms-keyring", "", "KMS key ring name (required when using --kms-key)")
	computeCmd.Flags().StringVar(&gceKmsLocation, "kms-location", "", "KMS key location/region (required when using --kms-key)")
	computeCmd.Flags().StringVar(&gceKmsProject, "kms-project", "", "KMS project ID (defaults to --project if not specified)")
	computeCmd.Flags().StringVar(&gceRegion, "region", "", "GCP region to search for instances (mutually exclusive with --zone)")
	computeCmd.Flags().StringVar(&gceZone, "zone", "", "GCP zone to search for instances (mutually exclusive with --region)")
	computeCmd.Flags().StringSliceVar(&gceInstances, "instances", nil, "Instance names (comma-separated) or '*' for all instances (required)")
	computeCmd.Flags().BoolVar(&gceAutoApprove, "auto-approve", false, "Skip interactive confirmations (default: false for safety)")
	computeCmd.Flags().IntVar(&gceMaxConcurrency, "max-concurrency", 5, "Maximum instances to process simultaneously (1-50, default: 5)")
	computeCmd.Flags().BoolVar(&gceRetainName, "retain-name", true, "Reuse original disk name by deleting original (default: true, irreversible)")
	computeCmd.Flags().Int64Var(&gceThroughput, "throughput", 150, "Disk throughput in MiB/s (applicable to hyperdisk types, default: 150)")
	computeCmd.Flags().Int64Var(&gceIops, "iops", 3000, "Disk IOPS limit (applicable to hyperdisk types, default: 3000)")

	_ = computeCmd.MarkFlagRequired("target-disk-type")
	_ = computeCmd.MarkFlagRequired("instances")
}

func validateComputeCmdFlags(cmd *cobra.Command, args []string) error {
	if projectID == "" { // projectID is from root persistent flag
		return migerrors.NewValidationError(
			migerrors.CodeValidationInput,
			"Project ID is required",
			"Parameter validation").
			WithContext("parameter", "--project").
			WithTroubleshooting(
				"Specify the project with --project flag",
				"Set GOOGLE_CLOUD_PROJECT environment variable",
				"Run 'gcloud config set project PROJECT_ID' to set default",
			)
	}

	if (gceZone == "" && gceRegion == "") || (gceZone != "" && gceRegion != "") {
		return migerrors.NewValidationError(
			migerrors.CodeValidationInput,
			"Either zone or region must be specified (but not both)",
			"Parameter validation").
			WithContext("zone", gceZone).
			WithContext("region", gceRegion).
			WithTroubleshooting(
				"Specify a zone with --zone (e.g., --zone us-central1-a)",
				"Or specify a region with --region (e.g., --region us-central1)",
				"Use 'gcloud compute zones list' to see available zones",
				"Do not specify both --zone and --region at the same time",
			)
	}

	if gceKmsKey != "" {
		if gceKmsKeyRing == "" || gceKmsLocation == "" {
			return migerrors.NewValidationError(
				migerrors.CodeValidationInput,
				"KMS key requires keyring and location",
				"KMS parameter validation").
				WithContext("kms_key", gceKmsKey).
				WithContext("kms_keyring", gceKmsKeyRing).
				WithContext("kms_location", gceKmsLocation).
				WithTroubleshooting(
					"Specify --kms-keyring with the KMS key ring name",
					"Specify --kms-location with the KMS location",
					"Or remove --kms-key to skip encryption",
				)
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
		return migerrors.NewValidationError(
			migerrors.CodeValidationInput,
			"Instance names are required",
			"Parameter validation").
			WithContext("parameter", "--instances").
			WithTroubleshooting(
				"Specify instance names with --instances flag (comma-separated)",
				"Example: --instances instance1,instance2",
				"Use 'gcloud compute instances list' to see available instances",
			)
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
		validationErr := migerrors.NewValidationError(
			migerrors.CodeValidationConfig,
			fmt.Sprintf("No instances are compatible with target disk type '%s'", gceTargetDiskType),
			"Instance validation").
			WithContext("target_disk_type", gceTargetDiskType).
			WithContext("total_instances", len(discoveredInstances)).
			WithContext("failed_instances", len(failedInstances)).
			WithTroubleshooting(
				"Check that the target disk type is supported by your instance machine types",
				"Review the validation failures above for specific compatibility issues",
				"Consider using a different target disk type that's more broadly supported",
				"Verify that your instances are running supported machine types",
			)
		return validationErr
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
