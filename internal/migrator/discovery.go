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
	logger.Starting("Discovery Phase")

	location := config.Location()
	logger.Infof("Listing detached disks in %s (Project: %s)", location, config.ProjectID)
	if config.LabelFilter != "" {
		logger.Infof("Applying label filter: %s", config.LabelFilter)
	}

	disksToMigrate, err := gcpClient.DiskClient.ListDetachedDisks(ctx, config.ProjectID, location, config.LabelFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list detached disks: %w", err)
	}

	if len(disksToMigrate) == 0 {
		logger.Info("No detached disks found matching the criteria.")
		return []*computepb.Disk{}, nil
	}

	logger.Infof("Found %d detached disk(s) matching criteria:", len(disksToMigrate))
	
	// Create table formatter
	table := utils.NewTableFormatter([]string{"#", "Name", "Zone", "Type", "Size"})
	for i, disk := range disksToMigrate {
		zone := "unknown"
		if disk.Zone != nil {
			parts := strings.Split(disk.GetZone(), "/")
			zone = parts[len(parts)-1]
		}

		table.AddRow([]string{
			fmt.Sprintf("%d", i+1),
			disk.GetName(),
			zone,
			getShortDiskTypeName(disk.GetType()),
			fmt.Sprintf("%d GB", disk.GetSizeGb()),
		})
	}

	logger.Info("\n" + table.String())

	// Generate migration summary
	totalSize := int64(0)
	for _, disk := range disksToMigrate {
		totalSize += disk.GetSizeGb()
	}
	
	// Build migration summary using message box
	summaryTitle := "Migration Summary"
	if config.DryRun {
		summaryTitle = "[DRY-RUN] Migration Plan"
	}
	
	summaryBox := utils.NewBox(utils.InfoMessage, summaryTitle).
		AddBullet(fmt.Sprintf("Target disk type: %s", config.TargetDiskType)).
		AddBullet(fmt.Sprintf("Disks to migrate: %d", len(disksToMigrate))).
		AddBullet(fmt.Sprintf("Total size: %d GB", totalSize)).
		AddBullet(fmt.Sprintf("Estimated time: ~%d minutes", len(disksToMigrate)*5)).
		AddBullet("Snapshots will be created: Yes")
	
	if config.RetainName {
		summaryBox.AddBullet("Original disks will be: Deleted (names retained)")
	} else {
		summaryBox.AddBullet("Original disks will be: Kept (new names will be generated)")
	}
	
	fmt.Println(summaryBox.Render())

	if config.DryRun {
		// Build dry-run actions using report builder for better structure
		actionsBuilder := utils.NewReportBuilder().
			Section("[DRY-RUN] Would perform the following actions:")
		
		for i, disk := range disksToMigrate {
			actionsBuilder.AddEmptyLine().
				AddLine(fmt.Sprintf("%d. Disk: %s", i+1, disk.GetName())).
				AddIndented(fmt.Sprintf("Create snapshot 'pd-migrate-%s-TIMESTAMP'", disk.GetName()), 1)
			
			if config.RetainName {
				actionsBuilder.
					AddIndented(fmt.Sprintf("Delete disk '%s'", disk.GetName()), 1).
					AddIndented(fmt.Sprintf("Create new disk '%s' with type '%s'", disk.GetName(), config.TargetDiskType), 1)
			} else {
				actionsBuilder.
					AddIndented(fmt.Sprintf("Create new disk '%s-migrated' with type '%s'", disk.GetName(), config.TargetDiskType), 1)
			}
			actionsBuilder.AddIndented("Clean up snapshot after verification", 1)
		}
		
		fmt.Println(actionsBuilder.Build())
		
		// Show info box for dry-run mode
		dryRunInfo := utils.Info("Dry-Run Mode", "No changes will be made in dry-run mode.")
		fmt.Println(dryRunInfo)
		
		logger.Success("Discovery phase completed (dry-run)")
		return disksToMigrate, nil
	}

	confirmed, err := utils.PromptForConfirmation(
		config.AutoApproveAll,
		fmt.Sprintf("migrate %d disk(s) to type '%s'", len(disksToMigrate), config.TargetDiskType),
		"Proceed with migration?",
	)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		logger.Info("Migration cancelled by user.")
		return []*computepb.Disk{}, nil
	}
	if !config.AutoApproveAll {
		logger.Info("User confirmed. Proceeding with migration.")
	}

	logger.Success("Discovery phase completed")
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
	logger.Starting("Instance Discovery")

	var discoveredInstances []*computepb.Instance
	var err error
	if len(config.Instances) > 0 && config.Instances[0] != "*" {
		// get instances by names
		for _, instanceName := range config.Instances {
			logger.Infof("Getting compute instance %s", instanceName)
			instance, err := gcpClient.ComputeClient.GetInstance(ctx, config.ProjectID, config.Zone, instanceName)
			if err != nil {
				return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", instanceName, config.Zone, err)
			}
			if instance != nil {
				discoveredInstances = append(discoveredInstances, instance)
			} else {
				logger.Warnf("Instance %s not found in zone %s", instanceName, config.Zone)
			}
		}
	} else if config.Zone != "" {
		logger.Infof("Listing instances in zone %s", config.Zone)
		discoveredInstances, err = listInstancesInZone(ctx, config.ProjectID, config.Zone, gcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in zone %s: %w", config.Zone, err)
		}
	} else if config.Region != "" {
		logger.Infof("Listing instances in region %s", config.Region)
		discoveredInstances, err = listInstancesInRegion(ctx, config.ProjectID, config.Region, gcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instances in region %s: %w", config.Region, err)
		}
	} else {
		return nil, fmt.Errorf("you must specify either a zone or a region for instance discovery")
	}

	if len(discoveredInstances) == 0 {
		logger.Info("No instances found matching the specified location.")
		return []*computepb.Instance{}, nil
	}
	logger.Info("\nDiscovered instances:")
	for i, instance := range discoveredInstances {
		logger.Infof("  %d. %s (Zone: %s)", i+1, instance.GetName(), utils.ExtractZoneName(instance.GetZone()))
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
