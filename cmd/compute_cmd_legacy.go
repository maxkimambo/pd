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
	gceTargetDiskTypeLegacy string
	gceLabelFilterLegacy    string
	gceKmsKeyLegacy         string
	gceKmsKeyRingLegacy     string
	gceKmsLocationLegacy    string
	gceKmsProjectLegacy     string
	gceRegionLegacy         string
	gceZoneLegacy           string
	gceInstancesLegacy      []string
	gceAutoApproveLegacy    bool
	gceMaxConcurrencyLegacy int
	gceRetainNameLegacy     bool
	gceThroughputLegacy     int64
	gceIopsLegacy           int64
)

var computeCmdLegacy = &cobra.Command{
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
	PreRunE: validateComputeCmdFlagsLegacy,
	RunE:    runGceConvertLegacy,
}

func init() {
	computeCmdLegacy.Flags().StringVarP(&projectID, "project", "p", "", "GCP Project ID (required)")
	computeCmdLegacy.Flags().StringVarP(&gceTargetDiskTypeLegacy, "target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, hyperdisk-balanced) (required)")
	computeCmdLegacy.Flags().StringVar(&gceLabelFilterLegacy, "label", "", "Label filter for disks in key=value format (optional)")
	computeCmdLegacy.Flags().StringVar(&gceKmsKeyLegacy, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	computeCmdLegacy.Flags().StringVar(&gceKmsKeyRingLegacy, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	computeCmdLegacy.Flags().StringVar(&gceKmsLocationLegacy, "kms-location", "", "KMS Key location (required if kms-key is set)")
	computeCmdLegacy.Flags().StringVar(&gceKmsProjectLegacy, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	computeCmdLegacy.Flags().StringVar(&gceRegionLegacy, "region", "", "GCP region (required if zone is not set)")
	computeCmdLegacy.Flags().StringVar(&gceZoneLegacy, "zone", "", "GCP zone (required if region is not set)")
	computeCmdLegacy.Flags().StringSliceVar(&gceInstancesLegacy, "instances", nil, "Comma-separated list of instance names, or '*' for all instances in the scope (required)")
	computeCmdLegacy.Flags().BoolVar(&gceAutoApproveLegacy, "auto-approve", false, "Skip all interactive prompts")
	computeCmdLegacy.Flags().IntVar(&gceMaxConcurrencyLegacy, "max-concurrency", 5, "Maximum number of disks/instances to process concurrently (1-50)")
	computeCmdLegacy.Flags().BoolVar(&gceRetainNameLegacy, "retain-name", true, "Reuse original disk name. If false, keep original and suffix new name.")
	computeCmdLegacy.Flags().Int64Var(&gceThroughputLegacy, "throughput", 150, "Throughput for the new disk in MiB/s (optional, default is 150)")
	computeCmdLegacy.Flags().Int64Var(&gceIopsLegacy, "iops", 3000, "IOPS for the new disk (optional, default is 3000)")
	computeCmdLegacy.MarkFlagRequired("target-disk-type")
	computeCmdLegacy.MarkFlagRequired("instances")
}

func validateComputeCmdFlagsLegacy(cmd *cobra.Command, args []string) error {

	if projectID == "" { // projectID is from root persistent flag
		return errors.New("required flag --project not set")
	}

	if (gceZoneLegacy == "" && gceRegionLegacy == "") || (gceZoneLegacy != "" && gceRegionLegacy != "") {
		return errors.New("exactly one of --zone or --region must be specified")
	}

	if gceKmsKeyLegacy != "" {
		if gceKmsKeyRingLegacy == "" || gceKmsLocationLegacy == "" {
			return errors.New("--kms-keyring and --kms-location are required when --kms-key is specified")
		}
		if gceKmsProjectLegacy == "" {
			gceKmsProjectLegacy = projectID
		}
	}

	if gceLabelFilterLegacy != "" && !strings.Contains(gceLabelFilterLegacy, "=") {
		return fmt.Errorf("invalid label format for --label: %s. Expected key=value", gceLabelFilterLegacy)
	}

	if gceMaxConcurrencyLegacy < 1 || gceMaxConcurrencyLegacy > 50 { // Adjusted max concurrency for instance operations
		return fmt.Errorf("--max-concurrency must be between 1 and 50, got %d", gceMaxConcurrencyLegacy)
	}

	if len(gceInstancesLegacy) == 0 {
		return errors.New("required flag --instances not set")
	}

	return nil
}

func runGceConvertLegacy(cmd *cobra.Command, args []string) error {
	// Set verbose to true if debug is enabled for backward compatibility
	if debug {
		verbose = true
	}
	logger.Setup(verbose, jsonLogs, quiet)

	logger.User.Starting("Starting disk migration process...")
	// trim leading/trailing whitespace from instance names
	for i, instance := range gceInstancesLegacy {
		gceInstancesLegacy[i] = strings.TrimSpace(instance)
	}

	config := migrator.Config{
		ProjectID:      projectID,
		TargetDiskType: gceTargetDiskTypeLegacy,
		LabelFilter:    gceLabelFilterLegacy,
		KmsKey:         gceKmsKeyLegacy,
		KmsKeyRing:     gceKmsKeyRingLegacy,
		KmsLocation:    gceKmsLocationLegacy,
		KmsProject:     gceKmsProjectLegacy,
		Region:         gceRegionLegacy,
		Zone:           gceZoneLegacy,
		AutoApproveAll: gceAutoApproveLegacy,
		Concurrency:    gceMaxConcurrencyLegacy,
		RetainName:     gceRetainNameLegacy,
		Debug:          debug,
		Instances:      gceInstancesLegacy,
		Throughput:     gceThroughputLegacy,
		Iops:           gceIopsLegacy,
	}
	logger.Op.Debugf("Configuration: %+v", config)
	logger.User.Infof("Project: %s", projectID)
	if gceZoneLegacy != "" {
		logger.User.Infof("Zone: %s", gceZoneLegacy)
	} else {
		logger.User.Infof("Region: %s", gceRegionLegacy)
	}
	if gceInstancesLegacy[0] == "*" {
		logger.User.Infof("Target: All instances in scope (%s)", config.Location())
	} else {
		logger.User.Infof("Target: %s", strings.Join(gceInstancesLegacy, ", "))
	}
	logger.User.Infof("Target disk type: %s", gceTargetDiskTypeLegacy)

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
	logger.User.Infof("Discovered %d instance(s) for migration.", len(discoveredInstances))
	logger.User.Info("--- Phase 2: Migration (GCE Attached Disks) ---")

	for _, instance := range discoveredInstances {
		logger.User.Infof("Processing instance: %s", *instance.Name)
		migrator.HandleInstanceDiskMigration(ctx, &config, instance, gcpClient)
	}

	// --- Placeholder for Cleanup Phase ---
	logger.User.Info("--- Phase 3: Cleanup (Snapshots) ---")
	// Similar to existing snapshot cleanup, but based on snapshots created during this process.
	logger.User.Warn("Snapshot cleanup logic for GCE conversion is not yet implemented.")

	// --- Placeholder for Reporting Phase ---
	logger.User.Info("--- Reporting ---")
	// Generate a summary of actions taken, successes, failures.
	logger.User.Warn("Reporting for GCE conversion is not yet implemented.")

	return nil
}
