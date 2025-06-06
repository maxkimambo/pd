package migrator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
)

// DiscoverDisks discovers GCP Compute Engine persistent disks based on the migration configuration.
func DiscoverDisks(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Disk, error) {
	logrus.Info("--- Phase 1: Discovery ---")

	location := config.Location()
	logrus.Infof("Listing detached disks in %s (Project: %s)", location, config.ProjectID)
	if config.LabelFilter != "" {
		logrus.Infof("Applying label filter: %s", config.LabelFilter)
	}

	disksToMigrate, err := gcpClient.ListDetachedDisks(ctx, config.ProjectID, location, config.LabelFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list detached disks: %w", err)
	}

	if len(disksToMigrate) == 0 {
		logrus.Info("No detached disks found matching the criteria.")
		return []*computepb.Disk{}, nil
	}

	logrus.Infof("Found %d detached disk(s) matching criteria:", len(disksToMigrate))
	var sb strings.Builder
	for i, disk := range disksToMigrate {
		zone := "unknown"
		if disk.Zone != nil {
			parts := strings.Split(disk.GetZone(), "/")
			zone = parts[len(parts)-1]
		}

		sb.WriteString(fmt.Sprintf("  %d. %s", i+1, disk.GetName()))
		sb.WriteString(fmt.Sprintf(" (Zone: %s", zone))
		sb.WriteString(fmt.Sprintf(", Type: %s", getShortDiskTypeName(disk.GetType())))
		sb.WriteString(fmt.Sprintf(", Size: %d GB)", disk.GetSizeGb()))
		sb.WriteString("----------------------\n")
	}

	logrus.Info(sb.String())

	if !config.AutoApproveAll {
		fmt.Printf("\nProceed with migrating these %d disk(s) to type '%s'? (yes/no): ", len(disksToMigrate), config.TargetDiskType)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read user confirmation: %w", err)
		}
		input = strings.ToLower(strings.TrimSpace(input))
		if input != "yes" {
			logrus.Info("Migration cancelled by user.")
			return []*computepb.Disk{}, nil
		}
		logrus.Info("User confirmed. Proceeding with migration.")
	} else {
		logrus.Info("Skipping user confirmation due to --auto-approve flag.")
	}

	logrus.Info("--- Discovery Phase Complete ---")
	return disksToMigrate, nil
}

func getShortDiskTypeName(typeURL string) string {
	if typeURL == "" {
		return "unknown"
	}
	parts := strings.Split(typeURL, "/")
	return parts[len(parts)-1]
}

// DiscoverInstances discovers GCP Compute Engine instances in a specified zone or region.
func DiscoverInstances(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	logrus.Info("--- Phase 1: Discovering Instances ---")

	var discoveredInstances []*computepb.Instance
	var err error

	// If a specific zone is provided, list instances only in that zone.
	if config.Zone != "" {
		logrus.Infof("Listing instances in zone %s (Project: %s)", config.Zone, config.ProjectID)
		discoveredInstances, err = listInstancesInZone(ctx, config.ProjectID, config.Zone, gcpClient) // Updated call
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in zone %s: %w", config.Zone, err)
		}
	} else if config.Region != "" {
		// If a region is provided, list all instances in that region by aggregating across its zones.
		logrus.Infof("Listing instances in region %s (Project: %s)", config.Region, config.ProjectID)
		discoveredInstances, err = listInstancesInRegion(ctx, config.ProjectID, config.Region, gcpClient) // Updated call
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in region %s: %w", config.Region, err)
		}
	} else {
		return nil, fmt.Errorf("you must specify either a zone or a region for instance discovery")
	}

	if len(discoveredInstances) == 0 {
		logrus.Info("No instances found matching the specified location.")
		return []*computepb.Instance{}, nil
	}

	logrus.Infof("Successfully discovered %d instance(s):", len(discoveredInstances))
	var sb strings.Builder
	for i, instance := range discoveredInstances {
		// Extract zone from full URL for cleaner display if needed
		zoneParts := strings.Split(instance.GetZone(), "/")
		displayZone := zoneParts[len(zoneParts)-1]
		sb.WriteString(fmt.Sprintf("  %d. %s (Zone: %s)\n", i+1, instance.GetName(), displayZone))
	}
	if sb.Len() > 0 {
		logrus.Info(sb.String())
	}

	return discoveredInstances, nil
}

// listInstancesInZone iterates and collects all instances from a single specified zone.
func listInstancesInZone(ctx context.Context, projectID, zone string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	// The iteration is now handled by gcpClient.ListInstancesInZone
	instances, err := gcpClient.ListInstancesInZone(ctx, projectID, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances in zone %s: %w", zone, err)
	}
	return instances, nil
}

// listInstancesInRegion iterates through all zones in a project and filters instances by the specified region.
// Note: This uses AggregatedList for efficiency.
func listInstancesInRegion(ctx context.Context, projectID, region string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	allInstances, err := gcpClient.AggregatedListInstances(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve aggregated instances list for project %s: %w", projectID, err)
	}

	var instancesInRegion []*computepb.Instance
	for _, instance := range allInstances {
		if instance.GetZone() != "" {
			// Zone URL is like projects/PROJECT_ID/zones/ZONE_NAME
			zoneParts := strings.Split(instance.GetZone(), "/")
			instanceZone := zoneParts[len(zoneParts)-1]
			if strings.HasPrefix(instanceZone, region) {
				instancesInRegion = append(instancesInRegion, instance)
			}
		}
	}
	return instancesInRegion, nil
}
