package dag

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/utils"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// DiscoveryTask wraps instance/disk discovery operations
type DiscoveryTask struct {
	*BaseTask
	config        *migrator.Config
	gcpClient     *gcp.Clients
	discoveredRes interface{} // will hold discovered instances or disks
	resourceType  string      // "instances" or "disks"
}

// NewDiscoveryTask creates a new discovery task
func NewDiscoveryTask(id string, config *migrator.Config, gcpClient *gcp.Clients, resourceType string) *DiscoveryTask {
	return &DiscoveryTask{
		BaseTask:     NewBaseTask(id, "Discovery", fmt.Sprintf("Discover %s", resourceType)),
		config:       config,
		gcpClient:    gcpClient,
		resourceType: resourceType,
	}
}

// Execute performs resource discovery
func (t *DiscoveryTask) Execute(ctx context.Context) error {
	switch t.resourceType {
	case "disks":
		disks, err := migrator.DiscoverDisks(ctx, t.config, t.gcpClient)
		if err != nil {
			return err
		}
		t.discoveredRes = disks
	case "instances":
		instances, err := migrator.DiscoverInstances(ctx, t.config, t.gcpClient)
		if err != nil {
			return err
		}
		t.discoveredRes = instances
	default:
		return fmt.Errorf("unknown resource type: %s", t.resourceType)
	}
	return nil
}

// Rollback is a no-op for discovery
func (t *DiscoveryTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// GetDiscoveredResources returns the discovered resources
func (t *DiscoveryTask) GetDiscoveredResources() interface{} {
	return t.discoveredRes
}

// SnapshotTask wraps snapshot creation operations
type SnapshotTask struct {
	*BaseTask
	projectID    string
	zone         string
	diskName     string
	snapshotName string
	gcpClient    *gcp.Clients
	config       *migrator.Config
	created      bool
}

// NewSnapshotTask creates a new snapshot task
func NewSnapshotTask(id, projectID, zone, diskName, snapshotName string, gcpClient *gcp.Clients, config *migrator.Config) *SnapshotTask {
	return &SnapshotTask{
		BaseTask:     NewBaseTask(id, "Snapshot", fmt.Sprintf("Create snapshot %s of disk %s", snapshotName, diskName)),
		projectID:    projectID,
		zone:         zone,
		diskName:     diskName,
		snapshotName: snapshotName,
		gcpClient:    gcpClient,
		config:       config,
		created:      false,
	}
}

// Execute creates a disk snapshot
func (t *SnapshotTask) Execute(ctx context.Context) error {
	kmsParams := t.config.PopulateKmsParams()
	
	// Get disk labels to include in snapshot
	disk, err := t.gcpClient.DiskClient.GetDisk(ctx, t.projectID, t.zone, t.diskName)
	if err != nil {
		return fmt.Errorf("failed to get disk for snapshot: %w", err)
	}
	
	err = t.gcpClient.SnapshotClient.CreateSnapshot(ctx, t.projectID, t.zone, t.diskName, t.snapshotName, kmsParams, disk.GetLabels())
	if err != nil {
		return err
	}
	
	t.created = true
	return nil
}

// Rollback is simplified - no operation needed
func (t *SnapshotTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// GetSnapshotName returns the created snapshot name
func (t *SnapshotTask) GetSnapshotName() string {
	return t.snapshotName
}

// InstanceStateTask wraps instance state operations
type InstanceStateTask struct {
	*BaseTask
	projectID     string
	zone          string
	instanceName  string
	action        string
	gcpClient     *gcp.Clients
	previousState string
	stateChanged  bool
}

// NewInstanceStateTask creates a new instance state task
func NewInstanceStateTask(id, projectID, zone, instanceName, action string, gcpClient *gcp.Clients) *InstanceStateTask {
	return &InstanceStateTask{
		BaseTask:     NewBaseTask(id, "InstanceState", fmt.Sprintf("%s instance %s", strings.ToUpper(action[:1])+action[1:], instanceName)),
		projectID:    projectID,
		zone:         zone,
		instanceName: instanceName,
		action:       action,
		gcpClient:    gcpClient,
	}
}

// Execute performs the instance state operation
func (t *InstanceStateTask) Execute(ctx context.Context) error {
	// Get current state first
	instance, err := t.gcpClient.ComputeClient.GetInstance(ctx, t.projectID, t.zone, t.instanceName)
	if err != nil {
		return err
	}
	
	t.previousState = instance.GetStatus()
	
	// Perform the requested action
	switch t.action {
	case "stop":
		if t.previousState == "RUNNING" {
			err = t.gcpClient.ComputeClient.StopInstance(ctx, t.projectID, t.zone, t.instanceName)
			if err != nil {
				return err
			}
			t.stateChanged = true
		}
	case "start":
		if t.previousState != "RUNNING" {
			err = t.gcpClient.ComputeClient.StartInstance(ctx, t.projectID, t.zone, t.instanceName)
			if err != nil {
				return err
			}
			t.stateChanged = true
		}
	default:
		return fmt.Errorf("unknown instance action: %s", t.action)
	}
	
	return nil
}

// Rollback is simplified - no operation needed
func (t *InstanceStateTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// DiskMigrationTask wraps disk migration operations
type DiskMigrationTask struct {
	*BaseTask
	projectID    string
	zone         string
	diskName     string
	newDiskName  string
	targetType   string
	snapshotName string
	gcpClient    *gcp.Clients
	config       *migrator.Config
	disk         *computepb.Disk
	migrated     bool
}

// NewDiskMigrationTask creates a new disk migration task
func NewDiskMigrationTask(id, projectID, zone, diskName, targetType, snapshotName string, gcpClient *gcp.Clients, config *migrator.Config, disk *computepb.Disk) *DiskMigrationTask {
	newDiskName := diskName
	if !config.RetainName {
		newDiskName = utils.AddSuffix(diskName, 4)
	}
	
	return &DiskMigrationTask{
		BaseTask:     NewBaseTask(id, "DiskMigration", fmt.Sprintf("Migrate disk %s to %s", diskName, targetType)),
		projectID:    projectID,
		zone:         zone,
		diskName:     diskName,
		newDiskName:  newDiskName,
		targetType:   targetType,
		snapshotName: snapshotName,
		gcpClient:    gcpClient,
		config:       config,
		disk:         disk,
		migrated:     false,
	}
}

// Execute performs the disk migration
func (t *DiskMigrationTask) Execute(ctx context.Context) error {
	// Delete original disk if retaining name
	if t.config.RetainName {
		err := t.gcpClient.DiskClient.DeleteDisk(ctx, t.projectID, t.zone, t.diskName)
		if err != nil {
			return fmt.Errorf("failed to delete original disk: %w", err)
		}
	}
	
	// Create new disk from snapshot
	newDiskLabels := t.disk.GetLabels()
	if newDiskLabels == nil {
		newDiskLabels = make(map[string]string)
	}
	newDiskLabels["migration"] = "success"
	
	storagePoolUrl := utils.GetStoragePoolURL(t.config.StoragePoolId, t.projectID, t.zone)
	err := t.gcpClient.DiskClient.CreateNewDiskFromSnapshot(
		ctx, t.projectID, t.zone, t.newDiskName, t.targetType, t.snapshotName,
		newDiskLabels, *t.disk.SizeGb, t.config.Iops, t.config.Throughput, storagePoolUrl)
	if err != nil {
		return fmt.Errorf("failed to create new disk from snapshot: %w", err)
	}
	
	t.migrated = true
	return nil
}

// Rollback is simplified - no operation needed
func (t *DiskMigrationTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// GetNewDiskName returns the name of the newly created disk
func (t *DiskMigrationTask) GetNewDiskName() string {
	return t.newDiskName
}

// DiskAttachmentTask wraps disk attachment/detachment operations
type DiskAttachmentTask struct {
	*BaseTask
	projectID    string
	zone         string
	instanceName string
	diskName     string
	deviceName   string
	action       string // "attach" or "detach"
	gcpClient    *gcp.Clients
	executed     bool
}

// NewDiskAttachmentTask creates a new disk attachment task
func NewDiskAttachmentTask(id, projectID, zone, instanceName, diskName, deviceName, action string, gcpClient *gcp.Clients) *DiskAttachmentTask {
	return &DiskAttachmentTask{
		BaseTask:     NewBaseTask(id, "DiskAttachment", fmt.Sprintf("%s disk %s to instance %s", strings.ToUpper(action[:1])+action[1:], diskName, instanceName)),
		projectID:    projectID,
		zone:         zone,
		instanceName: instanceName,
		diskName:     diskName,
		deviceName:   deviceName,
		action:       action,
		gcpClient:    gcpClient,
		executed:     false,
	}
}

// Execute performs the disk attachment/detachment operation
func (t *DiskAttachmentTask) Execute(ctx context.Context) error {
	switch t.action {
	case "attach":
		err := t.gcpClient.ComputeClient.AttachDisk(ctx, t.projectID, t.zone, t.instanceName, t.diskName, t.deviceName)
		if err != nil {
			return err
		}
	case "detach":
		err := t.gcpClient.ComputeClient.DetachDisk(ctx, t.projectID, t.zone, t.instanceName, t.deviceName)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown disk attachment action: %s", t.action)
	}
	
	t.executed = true
	return nil
}

// Rollback is simplified - no operation needed
func (t *DiskAttachmentTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// CleanupTask wraps cleanup operations
type CleanupTask struct {
	*BaseTask
	projectID    string
	resourceID   string
	resourceType string
	gcpClient    *gcp.Clients
}

// NewCleanupTask creates a new cleanup task
func NewCleanupTask(id, projectID, resourceType, resourceID string, gcpClient *gcp.Clients) *CleanupTask {
	return &CleanupTask{
		BaseTask:     NewBaseTask(id, "Cleanup", fmt.Sprintf("Clean up %s %s", resourceType, resourceID)),
		projectID:    projectID,
		resourceID:   resourceID,
		resourceType: resourceType,
		gcpClient:    gcpClient,
	}
}

// Execute performs the cleanup operation
func (t *CleanupTask) Execute(ctx context.Context) error {
	switch t.resourceType {
	case "snapshot":
		return t.gcpClient.SnapshotClient.DeleteSnapshot(ctx, t.projectID, t.resourceID)
	default:
		return fmt.Errorf("unknown resource type for cleanup: %s", t.resourceType)
	}
}

// Rollback is simplified - no operation needed
func (t *CleanupTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}