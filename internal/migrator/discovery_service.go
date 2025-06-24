package migrator

import (
	"context"
	"fmt"
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
)

// InstanceDiscovery provides instance discovery functionality for migration
type InstanceDiscovery struct {
	computeClient gcp.ComputeClientInterface
	diskClient    gcp.DiskClientInterface
}

// NewInstanceDiscovery creates a new InstanceDiscovery service
func NewInstanceDiscovery(computeClient gcp.ComputeClientInterface, diskClient gcp.DiskClientInterface) *InstanceDiscovery {
	return &InstanceDiscovery{
		computeClient: computeClient,
		diskClient:    diskClient,
	}
}

// DiscoverInstances discovers instances based on the provided configuration
func (d *InstanceDiscovery) DiscoverInstances(ctx context.Context, config *Config) ([]*InstanceMigration, error) {
	logger.User.Info("--- Phase 1: Instance Discovery ---")

	if len(config.Instances) > 0 {
		return d.DiscoverByNames(ctx, config)
	}

	if config.Zone != "" {
		return d.DiscoverByZone(ctx, config)
	}

	return nil, fmt.Errorf("no discovery criteria provided: specify instance names or zone")
}

// DiscoverByNames discovers instances by their specific names
func (d *InstanceDiscovery) DiscoverByNames(ctx context.Context, config *Config) ([]*InstanceMigration, error) {
	var migrations []*InstanceMigration

	logger.User.Infof("Discovering %d instances by name in zone %s", len(config.Instances), config.Zone)

	for _, name := range config.Instances {
		logger.User.Infof("Getting instance: %s", name)

		instance, err := d.computeClient.GetInstance(ctx, config.ProjectID, config.Zone, name)
		if err != nil {
			return nil, fmt.Errorf("failed to get instance %s in zone %s: %w", name, config.Zone, err)
		}

		migration, err := d.createInstanceMigration(ctx, instance, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create migration for instance %s: %w", name, err)
		}

		migrations = append(migrations, migration)
		logger.User.Infof("Successfully discovered instance: %s", name)
	}

	logger.User.Infof("Discovered %d instances for migration", len(migrations))
	return migrations, nil
}

// DiscoverByZone discovers all instances in the specified zone
func (d *InstanceDiscovery) DiscoverByZone(ctx context.Context, config *Config) ([]*InstanceMigration, error) {
	logger.User.Infof("Discovering instances in zone: %s", config.Zone)

	instances, err := d.computeClient.ListInstancesInZone(ctx, config.ProjectID, config.Zone)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances in zone %s: %w", config.Zone, err)
	}

	logger.User.Infof("Found %d instances in zone %s", len(instances), config.Zone)
	return d.createMigrationsFromInstances(ctx, instances, config)
}

// createMigrationsFromInstances creates migration objects from a list of instances
func (d *InstanceDiscovery) createMigrationsFromInstances(ctx context.Context, instances []*computepb.Instance, config *Config) ([]*InstanceMigration, error) {
	var migrations []*InstanceMigration

	for _, instance := range instances {
		migration, err := d.createInstanceMigration(ctx, instance, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create migration for instance %s: %w", instance.GetName(), err)
		}

		migrations = append(migrations, migration)
	}

	logger.User.Infof("Created %d instance migrations", len(migrations))
	return migrations, nil
}

// createInstanceMigration creates an InstanceMigration from a GCP instance
func (d *InstanceDiscovery) createInstanceMigration(ctx context.Context, instance *computepb.Instance, config *Config) (*InstanceMigration, error) {
	migration := &InstanceMigration{
		Instance:     instance,
		InitialState: getInstanceState(instance),
		Status:       MigrationStatusPending,
		Disks:        []*AttachedDiskInfo{},
		Results:      []DiskMigrationResult{},
		Errors:       []MigrationError{},
	}

	zoneName := utils.ExtractZoneName(instance.GetZone())
	logger.Op.WithFields(map[string]interface{}{
		"instance": instance.GetName(),
		"zone":     zoneName,
		"status":   instance.GetStatus(),
	}).Debug("Processing instance for migration")

	// Process attached disks
	for _, attachedDisk := range instance.GetDisks() {
		diskName := extractDiskNameFromSource(attachedDisk.GetSource())

		logger.Op.WithFields(map[string]interface{}{
			"instance": instance.GetName(),
			"disk":     diskName,
			"boot":     attachedDisk.GetBoot(),
		}).Debug("Processing attached disk")

		diskDetails, err := d.diskClient.GetDisk(ctx, config.ProjectID, zoneName, diskName)
		if err != nil {
			logger.Op.WithFields(map[string]interface{}{
				"instance": instance.GetName(),
				"disk":     diskName,
				"error":    err.Error(),
			}).Error("Failed to get disk details")

			// Add error to migration but continue with other disks
			migration.Errors = append(migration.Errors, MigrationError{
				Type:   ErrorTypePermanent,
				Phase:  PhaseDiscovery,
				Target: fmt.Sprintf("disk:%s", diskName),
				Cause:  err,
			})
			continue
		}

		diskInfo := &AttachedDiskInfo{
			AttachedDisk: attachedDisk,
			DiskDetails:  diskDetails,
			IsBoot:       attachedDisk.GetBoot(),
		}

		migration.Disks = append(migration.Disks, diskInfo)

		logger.Op.WithFields(map[string]interface{}{
			"instance": instance.GetName(),
			"disk":     diskName,
			"type":     diskDetails.GetType(),
			"size":     diskDetails.GetSizeGb(),
		}).Debug("Added disk to migration")
	}

	logger.Op.WithFields(map[string]interface{}{
		"instance":    instance.GetName(),
		"total_disks": len(migration.Disks),
		"errors":      len(migration.Errors),
	}).Info("Instance migration created")

	return migration, nil
}

// getInstanceState maps GCP instance status to our domain InstanceState
func getInstanceState(instance *computepb.Instance) InstanceState {
	switch instance.GetStatus() {
	case "RUNNING":
		return InstanceStateRunning
	case "TERMINATED":
		return InstanceStateStopped
	case "SUSPENDED":
		return InstanceStateSuspended
	default:
		return InstanceStateUnknown
	}
}

// extractDiskNameFromSource extracts the disk name from a GCP disk source URL
// Example: "projects/my-project/zones/us-west1-a/disks/my-disk" -> "my-disk"
func extractDiskNameFromSource(source string) string {
	if source == "" {
		return ""
	}
	parts := strings.Split(source, "/")
	if len(parts) == 0 {
		return source
	}
	return parts[len(parts)-1]
}
