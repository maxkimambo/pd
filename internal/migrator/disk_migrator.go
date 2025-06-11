package migrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// DiskMigratorInterface defines the interface for disk migration operations
type DiskMigratorInterface interface {
	MigrateInstanceDisks(ctx context.Context, migration *InstanceMigration) error
}

// DiskMigrator handles the migration of disks
type DiskMigrator struct {
	computeClient  gcp.ComputeClientInterface
	diskClient     gcp.DiskClientInterface
	snapshotClient gcp.SnapshotClientInterface
	config         *Config
}

// NewDiskMigrator creates a new disk migrator
func NewDiskMigrator(computeClient gcp.ComputeClientInterface, diskClient gcp.DiskClientInterface, snapshotClient gcp.SnapshotClientInterface, config *Config) *DiskMigrator {
	return &DiskMigrator{
		computeClient:  computeClient,
		diskClient:     diskClient,
		snapshotClient: snapshotClient,
		config:         config,
	}
}

// MigrateInstanceDisks migrates all disks for an instance
func (m *DiskMigrator) MigrateInstanceDisks(ctx context.Context, migration *InstanceMigration) error {
	instance := migration.Instance
	project := m.config.ProjectID
	zone := utils.ExtractZoneName(instance.GetZone())

	logger.User.Infof("Starting disk migration for instance %s", instance.GetName())
	migration.Status = MigrationStatusInProgress

	// Process each disk
	for _, diskInfo := range migration.Disks {
		logger.User.Infof("Processing disk: %s (boot: %t)", diskInfo.DiskDetails.GetName(), diskInfo.IsBoot)
		
		result := DiskMigrationResult{
			DiskInfo: diskInfo,
			Success:  false,
		}

		// Skip boot disks for now - they require special handling
		if diskInfo.IsBoot {
			logger.User.Infof("Skipping boot disk %s - not supported in this migration", diskInfo.DiskDetails.GetName())
			result.Success = true // Mark as successful skip
			result.Error = fmt.Errorf("boot disk migration not supported")
			migration.Results = append(migration.Results, result)
			continue
		}

		// Detach disk if needed
		if err := m.detachDiskIfNeeded(ctx, project, zone, instance, diskInfo); err != nil {
			logger.User.Errorf("Failed to detach disk %s: %v", diskInfo.DiskDetails.GetName(), err)
			result.Error = err
			migration.Results = append(migration.Results, result)
			continue
		}

		// Create snapshot
		snapshotName := fmt.Sprintf("%s-migration-%d", diskInfo.DiskDetails.GetName(), time.Now().Unix())
		if err := m.createDiskSnapshot(ctx, project, zone, diskInfo.DiskDetails.GetName(), snapshotName); err != nil {
			logger.User.Errorf("Failed to create snapshot for disk %s: %v", diskInfo.DiskDetails.GetName(), err)
			result.Error = err
			migration.Results = append(migration.Results, result)
			continue
		}

		// Create new disk from snapshot with desired configuration
		newDiskName := utils.AddSuffix(diskInfo.DiskDetails.GetName(), 4)
		if err := m.createNewDiskFromSnapshot(ctx, project, zone, newDiskName, snapshotName, diskInfo.DiskDetails); err != nil {
			logger.User.Errorf("Failed to create new disk %s: %v", newDiskName, err)
			result.Error = err
			migration.Results = append(migration.Results, result)
			continue
		}

		// Attach new disk
		deviceName := diskInfo.AttachedDisk.GetDeviceName()
		if err := m.attachNewDisk(ctx, project, zone, instance.GetName(), newDiskName, deviceName); err != nil {
			logger.User.Errorf("Failed to attach disk %s: %v", newDiskName, err)
			result.Error = err
			migration.Results = append(migration.Results, result)
			continue
		}

		// Mark as successful
		result.Success = true
		result.NewDiskLink = utils.GetDiskUrl(project, zone, newDiskName)
		migration.Results = append(migration.Results, result)
		logger.User.Successf("Successfully migrated disk %s to %s", diskInfo.DiskDetails.GetName(), newDiskName)
	}

	// Update migration status based on results
	m.updateMigrationStatus(migration)
	
	logger.User.Infof("Completed disk migration for instance %s", instance.GetName())
	return nil
}

// detachDiskIfNeeded detaches a disk if it's not a boot disk
func (m *DiskMigrator) detachDiskIfNeeded(ctx context.Context, project, zone string, instance *computepb.Instance, diskInfo *AttachedDiskInfo) error {
	// Skip detaching boot disks - they'll be handled differently
	if diskInfo.IsBoot {
		return nil
	}

	logger.Op.WithFields(map[string]interface{}{
		"instance":   instance.GetName(),
		"disk":       diskInfo.DiskDetails.GetName(),
		"deviceName": diskInfo.AttachedDisk.GetDeviceName(),
	}).Info("Detaching disk from instance")

	// Detach the disk
	err := m.computeClient.DetachDisk(ctx, project, zone, instance.GetName(), diskInfo.AttachedDisk.GetDeviceName())
	if err != nil {
		return fmt.Errorf("failed to detach disk %s: %w", diskInfo.DiskDetails.GetName(), err)
	}

	logger.Op.WithFields(map[string]interface{}{
		"instance": instance.GetName(),
		"disk":     diskInfo.DiskDetails.GetName(),
	}).Info("Successfully detached disk")

	return nil
}

// createDiskSnapshot creates a snapshot of a disk
func (m *DiskMigrator) createDiskSnapshot(ctx context.Context, project, zone, diskName, snapshotName string) error {
	labels := map[string]string{
		"managed-by":   "pd-migrate",
		"source-disk":  diskName,
		"phase":        "migration",
	}

	logger.Op.WithFields(map[string]interface{}{
		"disk":     diskName,
		"snapshot": snapshotName,
		"zone":     zone,
	}).Info("Creating snapshot")

	// Use KmsParams from config if available
	kmsParams := m.config.PopulateKmsParams()

	err := m.snapshotClient.CreateSnapshot(ctx, project, zone, diskName, snapshotName, kmsParams, labels)
	if err != nil {
		return fmt.Errorf("failed to create snapshot %s for disk %s: %w", snapshotName, diskName, err)
	}

	// Wait for snapshot to be ready
	if err := m.waitForSnapshotReady(ctx, project, snapshotName); err != nil {
		return fmt.Errorf("snapshot %s creation timeout: %w", snapshotName, err)
	}

	logger.Op.WithFields(map[string]interface{}{
		"disk":     diskName,
		"snapshot": snapshotName,
	}).Info("Snapshot created successfully")

	return nil
}

// createNewDiskFromSnapshot creates a new disk from a snapshot with the desired configuration
func (m *DiskMigrator) createNewDiskFromSnapshot(ctx context.Context, project, zone, newDiskName, snapshotName string, sourceDisk *computepb.Disk) error {
	// Preserve original labels and add migration info
	labels := make(map[string]string)
	for k, v := range sourceDisk.GetLabels() {
		labels[k] = v
	}
	labels["migrated-from"] = sourceDisk.GetName()
	labels["migration"] = "success"

	logger.Op.WithFields(map[string]interface{}{
		"newDisk":     newDiskName,
		"snapshot":    snapshotName,
		"sourceDisk":  sourceDisk.GetName(),
		"targetType":  m.config.TargetDiskType,
		"zone":        zone,
	}).Info("Creating new disk from snapshot")

	// Use the existing interface method with correct signature
	err := m.diskClient.CreateNewDiskFromSnapshot(
		ctx,
		project,
		zone,
		newDiskName,
		m.config.TargetDiskType,
		snapshotName,
		labels,
		sourceDisk.GetSizeGb(),
		m.config.Iops,
		m.config.Throughput,
		m.config.StoragePoolId,
	)

	if err != nil {
		return fmt.Errorf("failed to create new disk %s from snapshot %s: %w", newDiskName, snapshotName, err)
	}

	logger.Op.WithFields(map[string]interface{}{
		"newDisk":    newDiskName,
		"snapshot":   snapshotName,
		"sourceDisk": sourceDisk.GetName(),
	}).Info("New disk created successfully")

	return nil
}

// attachNewDisk attaches a new disk to the instance
func (m *DiskMigrator) attachNewDisk(ctx context.Context, project, zone, instanceName, diskName, deviceName string) error {
	logger.Op.WithFields(map[string]interface{}{
		"instance":   instanceName,
		"disk":       diskName,
		"deviceName": deviceName,
		"zone":       zone,
	}).Info("Attaching new disk to instance")

	err := m.computeClient.AttachDisk(ctx, project, zone, instanceName, diskName, deviceName)
	if err != nil {
		return fmt.Errorf("failed to attach disk %s to instance %s: %w", diskName, instanceName, err)
	}

	logger.Op.WithFields(map[string]interface{}{
		"instance": instanceName,
		"disk":     diskName,
	}).Info("Successfully attached disk to instance")

	return nil
}

// waitForSnapshotReady waits for a snapshot to be ready
func (m *DiskMigrator) waitForSnapshotReady(ctx context.Context, project, snapshotName string) error {
	logger.Op.WithFields(map[string]interface{}{
		"snapshot": snapshotName,
	}).Info("Waiting for snapshot to be ready")

	for attempts := 0; attempts < 30; attempts++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			snapshot, err := m.snapshotClient.GetSnapshot(ctx, project, snapshotName)
			if err != nil {
				logger.Op.WithFields(map[string]interface{}{
					"snapshot": snapshotName,
					"attempt":  attempts + 1,
					"error":    err.Error(),
				}).Warn("Failed to get snapshot status")
				continue
			}

			status := snapshot.GetStatus()
			logger.Op.WithFields(map[string]interface{}{
				"snapshot": snapshotName,
				"status":   status,
				"attempt":  attempts + 1,
			}).Debug("Snapshot status check")

			if status == "READY" {
				logger.Op.WithFields(map[string]interface{}{
					"snapshot": snapshotName,
					"attempts": attempts + 1,
				}).Info("Snapshot is ready")
				return nil
			}
		}
	}

	return fmt.Errorf("timed out waiting for snapshot %s to be ready after 30 attempts", snapshotName)
}

// updateMigrationStatus updates the overall migration status based on individual disk results
func (m *DiskMigrator) updateMigrationStatus(migration *InstanceMigration) {
	if len(migration.Results) == 0 {
		migration.Status = MigrationStatusFailed
		return
	}

	successCount := 0
	totalDisks := len(migration.Results)

	for _, result := range migration.Results {
		if result.Success {
			successCount++
		}
	}

	if successCount == totalDisks {
		migration.Status = MigrationStatusCompleted
	} else if successCount > 0 {
		migration.Status = MigrationStatusCompleted // Partial success still marked as completed
	} else {
		migration.Status = MigrationStatusFailed
	}

	logger.Op.WithFields(map[string]interface{}{
		"instance":     migration.Instance.GetName(),
		"totalDisks":   totalDisks,
		"successCount": successCount,
		"status":       migration.Status,
	}).Info("Migration status updated")
}

// extractProjectFromSelfLink extracts project ID from a GCP self link
func extractProjectFromSelfLink(selfLink string) string {
	if selfLink == "" {
		return ""
	}
	// Self link format: https://www.googleapis.com/compute/v1/projects/PROJECT_ID/...
	parts := strings.Split(selfLink, "/")
	for i, part := range parts {
		if part == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}