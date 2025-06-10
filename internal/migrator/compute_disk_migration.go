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
		diskName := attachedDisk.GetDeviceName()

		disk, err := gcpClient.DiskClient.GetDisk(ctx, config.ProjectID, zone, diskName)
		if err != nil {
			return fmt.Errorf("failed to get disk %s in zone %s: %w", diskName, zone, err)
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

// HandleInstanceDiskMigration coordinates the migration of non-boot disks for a given instance.
// For each instance:
//  1. Check the instance state (running or stopped).
//  2. If running, stop the instance.
//  3. Create an incremental snapshot of all disks attached to the instance.
//  4. Migrate non-boot disks:
//  5. If the instance was running, start it again after migration.

func HandleInstanceDiskMigration(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) error {
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
		logger.User.Infof("Instance %s in zone %s is running, stopping it before migration", instance.GetName(), zone)
		if err := gcpClient.ComputeClient.StopInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to stop instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}
	// Second snapshot before migration
	err = SnapshotInstanceDisks(ctx, config, instance, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to create final snapshot for instance %s in zone %s: %w", instance.GetName(), zone, err)
	}

	migrationResult, err := MigrateInstanceNonBootDisks(ctx, config, instance, gcpClient)

	if err != nil {
		return fmt.Errorf("failed to migrate non-boot disks for instance %s in zone %s: %w", instance.GetName(), zone, err)
	}

	for _, result := range migrationResult {
		if result.ErrorMessage != "" {
			logger.User.Errorf("Failed to migrate disk %s: %v  continuing with the next disk", result.DiskName, result.ErrorMessage)
		}
	}
	previousInstanceState, err := GetInstanceState(ctx, instance, gcpClient)
	if previousInstanceState == RUNNING_STATE && err == nil {
		logger.User.Infof("Instance %s in zone %s was running, starting it after migration", instance.GetName(), zone)
		if err := gcpClient.ComputeClient.StartInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to start instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}
	logger.User.Successf("All disks migrated successfully for instance %s", instance.GetName())
	return nil
}

func IncrementalSnapshotDisk(ctx context.Context, config *Config, disk *computepb.Disk, gcpClient *gcp.Clients) error {
	logger.User.Infof("Creating incremental snapshot for disk %s in zone %s", disk.GetName(), disk.GetZone())

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

	logger.User.Infof("Incremental snapshot %s created successfully for disk %s", snapshotName, disk.GetName())
	return nil
}
