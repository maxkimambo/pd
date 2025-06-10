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
		state = "RUNNING"
	} else {
		state = "STOPPED"
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

	var nonBootDisks []*computepb.Disk
	var results []MigrationResult

	for _, attachedDisk := range attachedDisks {
		if attachedDisk.GetBoot() {
			logger.User.Infof("Skipping boot disk %s", attachedDisk.GetDeviceName())
			continue
		}

		diskName := attachedDisk.GetDeviceName()
		disk, err := gcpClient.DiskClient.GetDisk(ctx, config.ProjectID, zone, diskName)
		if err != nil {
			logger.User.Errorf("Failed to get disk %s: %v", diskName, err)
			results = append(results, MigrationResult{
				DiskName:     diskName,
				Zone:         zone,
				Status:       "Failed: Disk Retrieval",
				ErrorMessage: fmt.Sprintf("Failed to get disk: %v", err),
			})
			continue
		}

		if disk == nil {
			logger.User.Warnf("Disk %s not found, skipping", diskName)
			continue
		}

		nonBootDisks = append(nonBootDisks, disk)
	}

	if len(nonBootDisks) == 0 {
		logger.User.Info("No non-boot disks found to migrate")
		return results, nil
	}

	logger.User.Infof("Found %d non-boot disks to migrate", len(nonBootDisks))

	for _, disk := range nonBootDisks {
		result := MigrateSingleDisk(ctx, config, gcpClient, disk)
		results = append(results, result)
	}

	successCount := 0
	for _, result := range results {
		if result.Status == "Success" {
			successCount++
		}
	}

	logger.User.Successf("Migrated %d/%d non-boot disks successfully for instance %s",
		successCount, len(results), instance.GetName())

	return results, nil
}

func MigrateInstanceDisks(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) error {
	// coordinate the disk migration process for the given instance
	defer removeInstanceState(instance)

	// check instance state using GetInstanceState
	state, err := GetInstanceState(ctx, instance, gcpClient)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}
	isRunning := state == "RUNNING"

	attachedDisks := instance.GetDisks()
	zone := utils.ExtractZoneName(instance.GetZone())
	if isRunning {
		logger.User.Infof("Instance %s in zone %s is running, stopping it before migration", instance.GetName(), zone)
		if err := gcpClient.ComputeClient.StopInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to stop instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}

	// start incremental snapshots
	for _, disk := range attachedDisks {
		if err := gcpClient.ComputeClient.DetachDisk(ctx, config.ProjectID, zone, instance.GetName(), disk.GetDeviceName()); err != nil {
			return fmt.Errorf("failed to detach disk %s from instance %s in zone %s: %w", disk.GetDeviceName(), instance.GetName(), zone, err)
		}
		// perform the migration for the detached disk
		logger.User.Infof("Migrating disk %s", disk.GetDeviceName())

		diskToMigrate, err := gcpClient.DiskClient.GetDisk(ctx, config.ProjectID, zone, disk.GetDeviceName())
		if err != nil {
			return fmt.Errorf("failed to get disk %s in zone %s: %w", disk.GetDeviceName(), zone, err)
		}

		if diskToMigrate != nil {
			MigrateSingleDisk(ctx, config, gcpClient, diskToMigrate)
		}
		// reattach the disks to the instance
		if err := gcpClient.ComputeClient.AttachDisk(ctx, config.ProjectID, zone, instance.GetName(), disk); err != nil {
			return fmt.Errorf("failed to reattach disk %s to instance %s in zone %s: %w", disk.GetDeviceName(), instance.GetName(), zone, err)
		}
	}

	// start the instance if it was initially running
	if isRunning {
		if err := gcpClient.ComputeClient.StartInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to start instance %s in zone %s: %w", instance.GetName(), zone, err)
		}
	}

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
