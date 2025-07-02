package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/taskmanager"
	"github.com/maxkimambo/pd/internal/validation"
	"github.com/maxkimambo/pd/internal/workflow"
	"github.com/spf13/cobra"
)


var computeCmd = &cobra.Command{
	Use:   "compute",
	Short: "Migrate attached persistent disks on GCE instances to a new disk type",
	Long: `Performs migration of persistent disks attached to specified GCE instances.

Identifies instances and their disks based on project, location (zone or region), and instance names.
For each targeted disk, it will:
1. Optionally stop the instance
2. Detach the disk
3. Create a snapshot (with optional KMS encryption)
4. Delete the original disk (if retaining name)
5. Recreate the disk from the snapshot with the target type
6. Attach the new disk to the instance
7. Optionally restart the instance
8. Clean up snapshots afterwards

Examples:
  pd migrate compute --project my-gcp-project --zone us-central1-a --instances vm1,vm2 --target-disk-type hyperdisk-balanced
  pd migrate compute --project my-gcp-project --zone us-central1-a --instances vm1 --target-disk-type pd-ssd --auto-approve
  pd migrate compute --project my-gcp-project --region us-central1 --instances "*" --target-disk-type hyperdisk-balanced
`,
	PreRunE: validateComputeCmdFlags,
	RunE:    runGceConvert,
}

func init() {
	computeCmd.Flags().StringP("target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, pd-balanced, hyperdisk-balanced) (required)")
	computeCmd.Flags().String("label", "", "Label filter for disks in key=value format (optional)")
	computeCmd.Flags().String("kms-key", "", "KMS key name for snapshot encryption (optional)")
	computeCmd.Flags().String("kms-keyring", "", "KMS keyring name (required if kms-key is set)")
	computeCmd.Flags().String("kms-location", "", "KMS key location (required if kms-key is set)")
	computeCmd.Flags().String("kms-project", "", "KMS project ID (defaults to --project if not set)")
	computeCmd.Flags().String("region", "", "GCP region for regional instances (use either --zone or --region, not both)")
	computeCmd.Flags().String("zone", "", "GCP zone for zonal instances (use either --zone or --region, not both)")
	computeCmd.Flags().StringSlice("instances", nil, "Comma-separated list of instance names, or '*' for all instances (required)")
	computeCmd.Flags().Bool("auto-approve", false, "Skip all interactive prompts and proceed with migration")
	computeCmd.Flags().Int("concurrency", 5, "Maximum number of concurrent instance/disk operations (1-50)")
	computeCmd.Flags().Bool("retain-name", true, "Reuse original disk name (deletes original). If false, creates new disk with suffix")

	if err := computeCmd.MarkFlagRequired("target-disk-type"); err != nil {
		logger.Errorf("Failed to mark target-disk-type as required: %s", err)
	}
	if err := computeCmd.MarkFlagRequired("instances"); err != nil {
		logger.Errorf("Failed to mark instances as required: %s", err)
	}
}

func validateComputeCmdFlags(cmd *cobra.Command, args []string) error {
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
	if err := validation.ValidateConcurrency(concurrency, 50); err != nil {
		return err
	}

	instances, _ := cmd.Flags().GetStringSlice("instances")
	if len(instances) == 0 {
		return fmt.Errorf("required flag --instances not set")
	}

	return nil
}

func runGceConvert(cmd *cobra.Command, args []string) error {
	logger.Starting("Starting disk migration process...")

	config, err := createMigrationConfig(cmd)
	if err != nil {
		return err
	}

	logger.Debugf("Configuration: %+v", config)
	logger.Infof("Project: %s", config.ProjectID)
	if config.Zone != "" {
		logger.Infof("Zone: %s", config.Zone)
	} else {
		logger.Infof("Region: %s", config.Region)
	}
	if config.Instances[0] == "*" {
		logger.Infof("Target: All instances in scope (%s)", config.Location())
	} else {
		logger.Infof("Target: %s", strings.Join(config.Instances, ", "))
	}
	logger.Infof("Target disk type: %s", config.TargetDiskType)

	ctx := context.Background()
	gcpClient, err := gcp.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GCP clients: %w", err)
	}
	defer gcpClient.Close()

	discoveredInstances, err := migrator.DiscoverInstances(ctx, config, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to discover GCE instances: %w", err)
	}
	logger.Infof("Discovered %d instance(s) for migration.", len(discoveredInstances))

	if len(discoveredInstances) == 0 {
		return fmt.Errorf("no instances found matching the specified criteria")
	}

	// Create migration manager
	manager := workflow.NewMigrationManager(gcpClient, config)

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
