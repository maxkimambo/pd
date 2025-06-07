package migrator

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
)

func MigrateInstanceDisks(ctx context.Context, config *Config, instance *computepb.Instance, gcpClient *gcp.Clients) error {
	// coordinate the disk migration process for the given instance

	// check instance state
	isRunning := gcpClient.InstanceClient.InstanceIsRunning(ctx, instance)

	attachedDisks := instance.GetDisks()
	zone := utils.ExtractZoneName(instance.GetZone())
	if isRunning {
		logger.User.Infof("Instance %s in zone %s is running, stopping it before migration", instance.GetName(), instance.GetZone())
		if err := gcpClient.InstanceClient.StopInstance(ctx, config.ProjectID, zone, instance.GetName()); err != nil {
			return fmt.Errorf("failed to stop instance %s in zone %s: %w", instance.GetName(), instance.GetZone(), err)
		}
	}

	// start incremental snapshots
	for _, disk := range attachedDisks {
		if err := gcpClient.InstanceClient.DetachDisk(ctx, config.ProjectID, zone, instance.GetName(), disk.GetDeviceName()); err != nil {
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
		if err := gcpClient.InstanceClient.AttachDisk(ctx, config.ProjectID, instance.GetZone(), instance.GetName(), disk); err != nil {
			return fmt.Errorf("failed to reattach disk %s to instance %s in zone %s: %w", disk.GetDeviceName(), instance.GetName(), instance.GetZone(), err)
		}
	}

	// start the instance if it was initially running
	if isRunning {
		if err := gcpClient.InstanceClient.StartInstance(ctx, config.ProjectID, instance.GetZone(), instance.GetName()); err != nil {
			return fmt.Errorf("failed to start instance %s in zone %s: %w", instance.GetName(), instance.GetZone(), err)
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
