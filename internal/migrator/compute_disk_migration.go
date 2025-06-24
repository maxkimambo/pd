package migrator

import (
	"context"
	"fmt"
	"sync"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
)

var (
	instanceStateMap = make(map[string]string)
	stateMapMutex    sync.RWMutex
	RUNNING_STATE    = "RUNNING"
	STOPPED_STATE    = "STOPPED"
)

func GetInstanceState(ctx context.Context, instance *computepb.Instance, gcpClient *gcp.Clients) (string, error) {
	instanceKey := fmt.Sprintf("%s/%s", utils.ExtractZoneName(instance.GetZone()), instance.GetName())

	stateMapMutex.RLock()
	if state, exists := instanceStateMap[instanceKey]; exists {
		stateMapMutex.RUnlock()
		return state, nil
	}
	stateMapMutex.RUnlock()

	isRunning := gcpClient.ComputeClient.InstanceIsRunning(ctx, instance)

	var state string
	if isRunning {
		state = RUNNING_STATE
	} else {
		state = STOPPED_STATE
	}

	stateMapMutex.Lock()
	instanceStateMap[instanceKey] = state
	stateMapMutex.Unlock()

	logger.User.Infof("Instance %s state: %s", instanceKey, state)
	return state, nil
}

func removeInstanceState(instance *computepb.Instance) {
	instanceKey := fmt.Sprintf("%s/%s", utils.ExtractZoneName(instance.GetZone()), instance.GetName())

	stateMapMutex.Lock()
	delete(instanceStateMap, instanceKey)
	stateMapMutex.Unlock()

	logger.Op.Debugf("Removed instance %s from state map", instanceKey)
}

func SnapshotInstanceDisks(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) error {
	logger.User.Infof("Creating snapshots for all disks attached to instance %s", instance.GetName())

	attachedDisks := instance.GetDisks()
	zone := utils.ExtractZoneName(instance.GetZone())

	for _, attachedDisk := range attachedDisks {
		if attachedDisk.GetBoot() {
			continue // Skip boot disks for snapshot creation
		}

		diskName := attachedDisk.GetDeviceName()

		disk, err := gcpClient.DiskClient.GetDisk(ctx, config.ProjectID, zone, diskName)
		if err != nil {
			logger.User.Errorf("Failed to get disk %s in zone %s: %v", diskName, zone, err)
			// return fmt.Errorf("failed to get disk %s in zone %s: %w", diskName, zone, err)
			continue
		}

		if disk == nil {
			logger.User.Warnf("Disk %s not found, skipping snapshot", diskName)
			continue
		}

		snapshotName := fmt.Sprintf("%s-snapshot", utils.AddSuffix(disk.GetName(), 3))
		kmsParams := &gcp.SnapshotKmsParams{
			KmsKey:      config.KmsKey,
			KmsKeyRing:  config.KmsKeyRing,
			KmsLocation: config.KmsLocation,
			KmsProject:  config.KmsProject,
		}

		logger.User.Snapshotf("Creating snapshot %s for disk %s", snapshotName, disk.GetName())
		labels := map[string]string{
			"managed-by": "pd-migrate",
			"instance":   instance.GetName(),
			"phase":      "pre-migration",
		}
		if err := gcpClient.SnapshotClient.CreateSnapshot(ctx, config.ProjectID, zone, disk.GetName(), snapshotName, kmsParams, labels); err != nil {
			return fmt.Errorf("failed to create snapshot for disk %s: %w", disk.GetName(), err)
		}

		logger.User.Successf("Snapshot %s created successfully for disk %s", snapshotName, disk.GetName())
	}

	logger.User.Success("All disk snapshots completed for instance " + instance.GetName())
	return nil
}

func MigrateInstanceNonBootDisks(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) ([]MigrationResult, error) {
	logger.User.Infof("Migrating non-boot disks for instance %s", instance.GetName())

	attachedDisks := instance.GetDisks()
	zone := utils.ExtractZoneName(instance.GetZone())

	var nonBootDisks []*computepb.AttachedDisk
	var results []MigrationResult

	for _, attachedDisk := range attachedDisks {
		if attachedDisk.GetBoot() {
			logger.User.Infof("Skipping boot disk %s", attachedDisk.GetDeviceName())
			continue
		}
		nonBootDisks = append(nonBootDisks, attachedDisk)
	}
	hasErrors := false
	for _, disk := range nonBootDisks {

		if err := gcpClient.ComputeClient.DetachDisk(ctx, config.ProjectID, zone, instance.GetName(), disk.GetDeviceName()); err != nil {
			return []MigrationResult{}, fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", disk.GetDeviceName(), instance.GetName(), zone, err)
		}
		// perform the migration for the detached disk
		logger.User.Infof("Migrating disk %s", disk.GetDeviceName())

		diskToMigrate, err := gcpClient.DiskClient.GetDisk(ctx, config.ProjectID, zone, disk.GetDeviceName())
		if err != nil {
			// record the error and continue with the next disk
			result := MigrationResult{
				DiskName:     disk.GetDeviceName(),
				Status:       "MigrationFailed",
				ErrorMessage: err.Error(),
			}
			results = append(results, result)
			logger.User.Errorf("Failed to migrate disk %s: %v  continuing with the next disk", disk.GetDeviceName(), err)
			hasErrors = true
			continue
		}
		var migrationResult MigrationResult
		if diskToMigrate != nil {
			migrationResult = MigrateSingleDisk(ctx, config, gcpClient, diskToMigrate)
			results = append(results, migrationResult)
		}
		newDisk := migrationResult.NewDiskName
		deviceName := disk.GetDeviceName()
		// reattach the disks to the instance
		if err := gcpClient.ComputeClient.AttachDisk(ctx, config.ProjectID, zone, instance.GetName(), newDisk, deviceName); err != nil {
			logger.User.Errorf("failed to reattach disk %s to instance %s in zone %s: %w", disk.GetDeviceName(), instance.GetName(), zone, err)
			// record the error and continue with the next disk
			result := MigrationResult{
				DiskName:     disk.GetDeviceName(),
				Zone:         zone,
				Status:       "Failed: Disk Attachment",
				ErrorMessage: err.Error(),
			}
			results = append(results, result)
			hasErrors = true
			continue
		}
	}

	if hasErrors {
		logger.User.Errorf("Some disks failed to migrate for instance %s. Check the logs for details.", instance.GetName())
	}

	return results, nil
}

// DAGOrchestrator defines the interface for DAG-based migration orchestration
type DAGOrchestrator interface {
	BuildMigrationDAG(ctx context.Context, instances []*computepb.Instance) (interface{}, error)
	ExecuteMigrationDAG(ctx context.Context, migrationDAG interface{}) (interface{}, error)
}

// ComputeDiskMigrator handles migration of disks attached to compute instances using DAG orchestration
type ComputeDiskMigrator struct {
	config       *Config
	gcpClient    *gcp.Clients
	orchestrator DAGOrchestrator
}

// NewComputeDiskMigrator creates a new compute disk migrator
func NewComputeDiskMigrator(config *Config, gcpClient *gcp.Clients, orchestrator DAGOrchestrator) *ComputeDiskMigrator {
	return &ComputeDiskMigrator{
		config:       config,
		gcpClient:    gcpClient,
		orchestrator: orchestrator,
	}
}

// MigrateInstanceDisks migrates all eligible disks for the specified instances using DAG orchestration
func (m *ComputeDiskMigrator) MigrateInstanceDisks(ctx context.Context, instances []*computepb.Instance) (interface{}, error) {
	if logger.User != nil {
		logger.User.Infof("Starting migration for %d instances", len(instances))
	}

	// Build the migration DAG
	migrationDAG, err := m.orchestrator.BuildMigrationDAG(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("failed to build migration DAG: %w", err)
	}

	// Execute the DAG
	if logger.User != nil {
		logger.User.Info("Executing migration tasks ...")
	}
	result, err := m.orchestrator.ExecuteMigrationDAG(ctx, migrationDAG)
	if err != nil {
		return result, fmt.Errorf("migration DAG execution failed: %w", err)
	}

	if logger.User != nil {
		logger.User.Successf("Migration completed successfully for %d instances", len(instances))
	}
	return result, nil
}

// HandleInstanceDiskMigration coordinates the migration of non-boot disks for a given instance.
// This method maintains backward compatibility with the existing linear workflow
// For now, it falls back to the original implementation until we can fully integrate the DAG orchestrator
func HandleInstanceDiskMigration(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) error {
	// For backward compatibility, use the original linear workflow for now
	// This avoids the import cycle while maintaining existing functionality

	// coordinate the disk migration process for the given instance
	defer removeInstanceState(instance)

	// check instance state using GetInstanceState
	state, err := GetInstanceState(ctx, instance, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}
	isRunning := state == RUNNING_STATE

	zone := utils.ExtractZoneName(instance.GetZone())
	// Perform incremental snapshot before migration
	err = SnapshotInstanceDisks(ctx, config, instance, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to create snapshot for instance %s in zone %s: %w", instance.GetName(), zone, err)
	}

	if isRunning {
		if logger.User != nil {
			logger.User.Infof("Instance %s in zone %s is running, stopping it before migration", instance.GetName(), zone)
		}
		if err := gcpClient.ComputeClient.StopInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to stop instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}
	migrationResult, err := MigrateInstanceNonBootDisks(ctx, config, instance, gcpClient)

	if err != nil {
		return fmt.Errorf("failed to migrate non-boot disks for instance %s in zone %s: %w", instance.GetName(), zone, err)
	}

	for _, result := range migrationResult {
		if result.ErrorMessage != "" {
			if logger.User != nil {
				logger.User.Errorf("Failed to migrate disk %s: %v  continuing with the next disk", result.DiskName, result.ErrorMessage)
			}
		}
	}
	previousInstanceState, err := GetInstanceState(ctx, instance, gcpClient)
	if previousInstanceState == RUNNING_STATE && err == nil {
		if logger.User != nil {
			logger.User.Infof("Instance %s in zone %s was running, starting it after migration", instance.GetName(), zone)
		}
		if err := gcpClient.ComputeClient.StartInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to start instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}
	if logger.User != nil {
		logger.User.Successf("All disks migrated successfully for instance %s", instance.GetName())
	}
	return nil
}

// MigrateAllInstanceDisks discovers and migrates disks for all matching instances using DAG orchestration
func (m *ComputeDiskMigrator) MigrateAllInstanceDisks(ctx context.Context) (interface{}, error) {
	// Discover instances first using proper arguments
	discovery := NewInstanceDiscovery(m.gcpClient.ComputeClient, m.gcpClient.DiskClient)
	instances, err := discovery.DiscoverInstances(ctx, m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to discover instances: %w", err)
	}

	if len(instances) == 0 {
		if logger.User != nil {
			logger.User.Info("No instances found matching the criteria")
		}
		// Return a simple success indicator
		return map[string]interface{}{"success": true}, nil
	}

	// Convert InstanceMigration to computepb.Instance
	instanceList := make([]*computepb.Instance, len(instances))
	for i, instMig := range instances {
		instanceList[i] = instMig.Instance
	}

	// Use DAG orchestration for migration
	return m.MigrateInstanceDisks(ctx, instanceList)
}

func IncrementalSnapshotDisk(ctx context.Context, config *Config, disk *computepb.Disk, gcpClient *gcp.Clients) error {
	if logger.User != nil {
		logger.User.Infof("Creating incremental snapshot for disk %s in zone %s", disk.GetName(), disk.GetZone())
	}

	snapshotName := fmt.Sprintf("%s-snapshot", utils.AddSuffix(disk.GetName(), 3))
	kmsParams := &gcp.SnapshotKmsParams{
		KmsKey:      config.KmsKey,
		KmsKeyRing:  config.KmsKeyRing,
		KmsLocation: config.KmsLocation,
		KmsProject:  config.KmsProject,
	}

	if err := gcpClient.SnapshotClient.CreateSnapshot(ctx, config.ProjectID, disk.GetZone(), disk.GetName(), snapshotName, kmsParams, nil); err != nil {
		return fmt.Errorf("failed to create incremental snapshot for disk %s: %w", disk.GetName(), err)
	}

	if logger.User != nil {
		logger.User.Infof("Incremental snapshot %s created successfully for disk %s", snapshotName, disk.GetName())
	}
	return nil
}
