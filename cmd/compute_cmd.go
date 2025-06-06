package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"

	// "github.com/maxkimambo/pd/internal/migrator" // Placeholder for future GCE migrator logic
	"github.com/sirupsen/logrus"
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
	gceInstances      string
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
pd migrate compute --project my-gcp-project --zone us-central1-a --instances my-instance-1,my-instance-2 --target-disk-type pd-ssd
pd migrate compute --project my-gcp-project --region us-central1 --instances "*" --target-disk-type hyperdisk-balanced --auto-approve
`,
	PreRunE: validateComputeCmdFlags,
	RunE:    runGceConvert,
}

func init() {

	computeCmd.Flags().StringVarP(&gceTargetDiskType, "target-disk-type", "t", "", "Target disk type (e.g., pd-ssd, hyperdisk-balanced) (required)")
	computeCmd.Flags().StringVar(&gceLabelFilter, "label", "", "Label filter for disks in key=value format (optional)")
	computeCmd.Flags().StringVar(&gceKmsKey, "kms-key", "", "KMS Key name for snapshot encryption (optional)")
	computeCmd.Flags().StringVar(&gceKmsKeyRing, "kms-keyring", "", "KMS KeyRing name (required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceKmsLocation, "kms-location", "", "KMS Key location (required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceKmsProject, "kms-project", "", "KMS Project ID (defaults to --project if not set, required if kms-key is set)")
	computeCmd.Flags().StringVar(&gceRegion, "region", "", "GCP region (required if zone is not set)")
	computeCmd.Flags().StringVar(&gceZone, "zone", "", "GCP zone (required if region is not set)")
	computeCmd.Flags().StringVarP(&gceInstances, "instances", "i", "*", "Comma-separated list of instance names, or '*' for all instances in the scope (required)")
	computeCmd.Flags().BoolVar(&gceAutoApprove, "auto-approve", false, "Skip all interactive prompts")
	computeCmd.Flags().IntVar(&gceMaxConcurrency, "max-concurrency", 5, "Maximum number of disks/instances to process concurrently (1-50)")
	computeCmd.Flags().BoolVar(&gceRetainName, "retain-name", true, "Reuse original disk name. If false, keep original and suffix new name.")

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

	if gceInstances == "" {
		return errors.New("required flag --instances not set")
	}

	return nil
}

func runGceConvert(cmd *cobra.Command, args []string) error {
	logger.Setup(debug) // debug is from root persistent flag

	logrus.Info("Starting disk conversion process...")

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
		Debug:          debug,
	}

	logrus.Infof("Project: %s", projectID)
	if gceZone != "" {
		logrus.Infof("Zone: %s", gceZone)
	} else {
		logrus.Infof("Region: %s", gceRegion)
	}
	logrus.Infof("Instances: %s", gceInstances)
	logrus.Infof("Target Disk Type: %s", gceTargetDiskType)
	// ... log other parameters ...

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
	logrus.Infof("Discovered %d instance(s) for migration.", len(discoveredInstances))

	// 1. Parse gceInstances: if "*", list all instances in scope. Otherwise, use provided names.
	// 2. For each instance, get its details, including attached disks.
	// 3. Filter disks based on gceLabelFilter (if provided).
	// 4. Present list to user for confirmation (unless --yes or --auto-approve).
	// logrus.Warn("Discovery logic for GCE instances and their disks is not yet implemented.")
	// discoveredInstanceDisks := []string{}                        // Placeholder
	// if len(discoveredInstanceDisks) == 0 && gceInstances != "" { // Simulate finding nothing if not implemented
	// 	logrus.Info("No instances/disks matching criteria found (or discovery not implemented). Exiting.")
	// 	// return nil // In a real scenario, might return error or specific message
	// }

	// --- Placeholder for Migration Phase ---
	logrus.Info("--- Phase 2: Migration (GCE Attached Disks) ---")
	// For each disk:
	//  a. Stop instance (optional, confirm with user).
	//  b. Detach disk.
	//  c. Create snapshot.
	//  d. Delete old disk (if retainName).
	//  e. Create new disk from snapshot with target type.
	//  f. Attach new disk.
	//  g. Start instance (if stopped).
	logrus.Warn("Migration logic for GCE attached disks is not yet implemented.")
	// migrationResults := []migrator.MigrationResult{} // Placeholder

	// --- Placeholder for Cleanup Phase ---
	logrus.Info("--- Phase 3: Cleanup (Snapshots) ---")
	// Similar to existing snapshot cleanup, but based on snapshots created during this process.
	logrus.Warn("Snapshot cleanup logic for GCE conversion is not yet implemented.")

	// --- Placeholder for Reporting Phase ---
	logrus.Info("--- Reporting ---")
	// Generate a summary of actions taken, successes, failures.
	logrus.Warn("Reporting for GCE conversion is not yet implemented.")

	logrus.Info("GCE attached disk conversion process command structure is set up. Core logic pending implementation.")
	return nil
}
