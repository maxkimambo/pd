package migrator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/utils"

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

	disksToMigrate, err := gcpClient.DiskClient.ListDetachedDisks(ctx, config.ProjectID, location, config.LabelFilter)
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

func DiscoverInstances(ctx context.Context, config *Config, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	logrus.Info("--- Phase 1: Discovering Instances ---")

	var discoveredInstances []*computepb.Instance
	var err error
	if len(config.Instances) > 0 && config.Instances[0] != "*" {
		// get instances by names
		for _, instanceName := range config.Instances {
			logrus.Infof("Getting compute instance %s", instanceName)
			instance, err := gcpClient.GetInstance(ctx, config.ProjectID, config.Zone, instanceName)
			if err != nil {
				return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, config.Zone, err)
			}
			if instance != nil {
				discoveredInstances = append(discoveredInstances, instance)
			} else {
				logrus.Warnf("Instance %s not found in zone %s", instanceName, config.Zone)
			}
		}
	} else if config.Zone != "" {
		logrus.Infof("Listing instances in zone %s", config.Zone)
		discoveredInstances, err = listInstancesInZone(ctx, config.ProjectID, config.Zone, gcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in zone %s: %w", config.Zone, err)
		}
	} else if config.Region != "" {
		logrus.Infof("Listing instances in region %s", config.Region)
		discoveredInstances, err = listInstancesInRegion(ctx, config.ProjectID, config.Region, gcpClient)
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
	var sb strings.Builder
	sb.WriteString("\n")
	for _, instance := range discoveredInstances {
		logrus.Infof("\t %s (Zone: %s)", instance.GetName(), utils.ExtractZoneName(instance.GetZone()))
	}
	return discoveredInstances, nil
}

func listInstancesInZone(ctx context.Context, projectID, zone string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {

	instances, err := gcpClient.ListInstancesInZone(ctx, projectID, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances in zone %s: %w", zone, err)
	}
	return instances, nil
}

func listInstancesInRegion(ctx context.Context, projectID, region string, gcpClient *gcp.Clients) ([]*computepb.Instance, error) {
	allInstances, err := gcpClient.AggregatedListInstances(ctx, projectID)
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
