package migrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// DiscoverDisks discovers GCP Compute Engine persistent disks based on the migration configuration.
func DiscoverDisks(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Disk, error) {
	logger.User.Info("--- Phase 1: Discovery ---")

	location := config.Location()
	logger.User.Infof("Listing detached disks in %s (Project: %s)", location, config.ProjectID)
	if config.LabelFilter != "" {
		logger.User.Infof("Applying label filter: %s", config.LabelFilter)
	}

	disksToMigrate, err := gcpClient.DiskClient.ListDetachedDisks(ctx, config.ProjectID, location, config.LabelFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list detached disks: %w", err)
	}

	if len(disksToMigrate) == 0 {
		logger.User.Info("No detached disks found matching the criteria.")
		return []*computepb.Disk{}, nil
	}

	logger.User.Infof("Found %d detached disk(s) matching criteria:", len(disksToMigrate))
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

	logger.User.Info(sb.String())

	confirmed, err := utils.PromptForConfirmation(
		config.AutoApproveAll,
		fmt.Sprintf("migrate %d disk(s) to type '%s'", len(disksToMigrate), config.TargetDiskType),
		"This will create snapshots and recreate disks",
	)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		logger.User.Info("Migration cancelled by user.")
		return []*computepb.Disk{}, nil
	}
	if !config.AutoApproveAll {
		logger.User.Info("User confirmed. Proceeding with migration.")
	}

	logger.User.Info("--- Discovery Phase Complete ---")
	return disksToMigrate, nil
}

func getShortDiskTypeName(typeURL string) string {
	if typeURL == "" {
		return "unknown"
	}
	parts := strings.Split(typeURL, "/")
	return parts[len(parts)-1]
}

func DiscoverInstances(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	logger.User.Info("--- Phase 1: Discovering Instances ---")

	var discoveredInstances []*computepb.Instance
	var err error
	if len(config.Instances) > 0 && config.Instances[0] != "*" {
		// get instances by names
		for _, instanceName := range config.Instances {
			logger.User.Infof("Getting compute instance %s", instanceName)
			instance, err := gcpClient.ComputeClient.GetInstance(ctx, config.ProjectID, config.Zone, instanceName)
			if err != nil {
				return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, config.Zone, err)
			}
			if instance != nil {
				discoveredInstances = append(discoveredInstances, instance)
			} else {
				logger.User.Warnf("Instance %s not found in zone %s", instanceName, config.Zone)
			}
		}
	} else if config.Zone != "" {
		logger.User.Infof("Listing instances in zone %s", config.Zone)
		discoveredInstances, err = listInstancesInZone(ctx, config.ProjectID, config.Zone, gcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in zone %s: %w", config.Zone, err)
		}
	} else if config.Region != "" {
		logger.User.Infof("Listing instances in region %s", config.Region)
		discoveredInstances, err = listInstancesInRegion(ctx, config.ProjectID, config.Region, gcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in region %s: %w", config.Region, err)
		}
	} else {
		return nil, fmt.Errorf("you must specify either a zone or a region for instance discovery")
	}

	if len(discoveredInstances) == 0 {
		logger.User.Info("No instances found matching the specified location.")
		return []*computepb.Instance{}, nil
	}
	var sb strings.Builder
	sb.WriteString("\n")
	for _, instance := range discoveredInstances {
		logger.User.Infof("\t %s (Zone: %s)", instance.GetName(), utils.ExtractZoneName(instance.GetZone()))
	}
	return discoveredInstances, nil
}

func listInstancesInZone(ctx context.Context, projectID, zone string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {

	instances, err := gcpClient.ComputeClient.ListInstancesInZone(ctx, projectID, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances in zone %s: %w", zone, err)
	}
	return instances, nil
}

func listInstancesInRegion(ctx context.Context, projectID, region string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	allInstances, err := gcpClient.ComputeClient.AggregatedListInstances(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve aggregated instances list for project %s: %w", projectID, err)
	}

	var instancesInRegion []*computepb.Instance
	for _, instance := range allInstances {
		if instance.GetZone() != "" {

			zoneParts := strings.Split(instance.GetZone(), "/")
			instanceZone := zoneParts[len(zoneParts)-1]
			if strings.HasPrefix(instanceZone, region) {
				instancesInRegion = append(instancesInRegion, instance)
			}
		}
	}
	return instancesInRegion, nil
}
