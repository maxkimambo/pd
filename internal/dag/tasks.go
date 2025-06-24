package dag

import (
	"context"
	"fmt"
	"strings"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/utils"
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
	name := fmt.Sprintf("Discover %s", resourceType)
	return &DiscoveryTask{
		BaseTask:     NewBaseTask(id, name, "Discovery"),
		config:       config,
		gcpClient:    gcpClient,
		resourceType: resourceType,
	}
}

// Execute performs resource discovery
func (t *DiscoveryTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()

	var err error
	switch t.resourceType {
	case "disks":
		disks, discErr := migrator.DiscoverDisks(ctx, t.config, t.gcpClient)
		if discErr != nil {
			result.MarkFailed(discErr)
			return result, discErr
		}
		t.discoveredRes = disks
		result.AddMetric("disks_discovered", len(disks))
	case "instances":
		instances, discErr := migrator.DiscoverInstances(ctx, t.config, t.gcpClient)
		if discErr != nil {
			result.MarkFailed(discErr)
			return result, discErr
		}
		t.discoveredRes = instances
		result.AddMetric("instances_discovered", len(instances))
	default:
		err = fmt.Errorf("unknown resource type: %s", t.resourceType)
		result.MarkFailed(err)
		return result, err
	}

	result.MarkCompleted()
	return result, nil
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
	sessionID    string
	cleanupAfter time.Duration
	created      bool
}

// NewSnapshotTask creates a new snapshot task
func NewSnapshotTask(id, projectID, zone, diskName, snapshotName string, gcpClient *gcp.Clients, config *migrator.Config) *SnapshotTask {
	name := fmt.Sprintf("Create snapshot %s of disk %s", snapshotName, diskName)
	return &SnapshotTask{
		BaseTask:     NewBaseTask(id, name, "Snapshot"),
		projectID:    projectID,
		zone:         zone,
		diskName:     diskName,
		snapshotName: snapshotName,
		gcpClient:    gcpClient,
		config:       config,
		sessionID:    "",             // Will be set by the task factory or orchestrator
		cleanupAfter: 24 * time.Hour, // Default cleanup after 24 hours
		created:      false,
	}
}

// NewSnapshotTaskWithSession creates a new snapshot task with session tracking
func NewSnapshotTaskWithSession(id, projectID, zone, diskName, snapshotName, sessionID string, gcpClient *gcp.Clients, config *migrator.Config, cleanupAfter time.Duration) *SnapshotTask {
	name := fmt.Sprintf("Create snapshot %s of disk %s", snapshotName, diskName)
	return &SnapshotTask{
		BaseTask:     NewBaseTask(id, name, "Snapshot"),
		projectID:    projectID,
		zone:         zone,
		diskName:     diskName,
		snapshotName: snapshotName,
		gcpClient:    gcpClient,
		config:       config,
		sessionID:    sessionID,
		cleanupAfter: cleanupAfter,
		created:      false,
	}
}

// Execute creates a disk snapshot
func (t *SnapshotTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("disk_name", t.diskName)
	result.AddMetadata("snapshot_name", t.snapshotName)
	result.AddMetadata("project_id", t.projectID)
	result.AddMetadata("zone", t.zone)

	kmsParams := t.config.PopulateKmsParams()

	// Use enhanced snapshot creation if session ID is available
	if t.sessionID != "" && t.cleanupAfter > 0 {
		// Create snapshot metadata
		metadata := gcp.NewSnapshotMetadata(t.sessionID, t.GetID(), t.diskName, t.cleanupAfter)

		// Get disk labels to include in snapshot metadata
		disk, err := t.gcpClient.DiskClient.GetDisk(ctx, t.projectID, t.zone, t.diskName)
		if err != nil {
			result.MarkFailed(fmt.Errorf("failed to get disk for snapshot: %w", err))
			return result, err
		}

		// Add disk labels to metadata
		if disk.GetLabels() != nil {
			for k, v := range disk.GetLabels() {
				metadata.Labels[k] = v
			}
		}

		err = t.gcpClient.SnapshotClient.CreateSnapshotWithMetadata(ctx, t.projectID, t.zone, t.diskName, t.snapshotName, kmsParams, metadata)
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
		result.AddMetric("with_metadata", true)
	} else {
		// Fallback to legacy snapshot creation
		disk, err := t.gcpClient.DiskClient.GetDisk(ctx, t.projectID, t.zone, t.diskName)
		if err != nil {
			result.MarkFailed(fmt.Errorf("failed to get disk for snapshot: %w", err))
			return result, err
		}

		err = t.gcpClient.SnapshotClient.CreateSnapshot(ctx, t.projectID, t.zone, t.diskName, t.snapshotName, kmsParams, disk.GetLabels())
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
		result.AddMetric("with_metadata", false)
	}

	t.created = true
	result.AddMetric("snapshot_created", true)
	result.MarkCompleted()
	return result, nil
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
	name := fmt.Sprintf("%s instance %s", strings.ToUpper(action[:1])+action[1:], instanceName)
	return &InstanceStateTask{
		BaseTask:     NewBaseTask(id, name, "InstanceState"),
		projectID:    projectID,
		zone:         zone,
		instanceName: instanceName,
		action:       action,
		gcpClient:    gcpClient,
	}
}

// Execute performs the instance state operation
func (t *InstanceStateTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("instance_name", t.instanceName)
	result.AddMetadata("action", t.action)
	result.AddMetadata("project_id", t.projectID)
	result.AddMetadata("zone", t.zone)

	// Get current state first
	instance, err := t.gcpClient.ComputeClient.GetInstance(ctx, t.projectID, t.zone, t.instanceName)
	if err != nil {
		result.MarkFailed(err)
		return result, err
	}

	t.previousState = instance.GetStatus()
	result.AddMetadata("previous_state", t.previousState)

	// Perform the requested action
	switch t.action {
	case "stop":
		if t.previousState == "RUNNING" {
			err = t.gcpClient.ComputeClient.StopInstance(ctx, t.projectID, t.zone, t.instanceName)
			if err != nil {
				result.MarkFailed(err)
				return result, err
			}
			t.stateChanged = true
			result.AddMetric("state_changed", true)
		} else {
			result.AddMetric("state_changed", false)
		}
	case "start":
		if t.previousState != "RUNNING" {
			err = t.gcpClient.ComputeClient.StartInstance(ctx, t.projectID, t.zone, t.instanceName)
			if err != nil {
				result.MarkFailed(err)
				return result, err
			}
			t.stateChanged = true
			result.AddMetric("state_changed", true)
		} else {
			result.AddMetric("state_changed", false)
		}
	default:
		err = fmt.Errorf("unknown instance action: %s", t.action)
		result.MarkFailed(err)
		return result, err
	}

	result.MarkCompleted()
	return result, nil
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
		BaseTask:     NewBaseTask(id, fmt.Sprintf("Migrate disk %s to %s", diskName, targetType), "DiskMigration"),
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
func (t *DiskMigrationTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("disk_name", t.diskName)
	result.AddMetadata("new_disk_name", t.newDiskName)
	result.AddMetadata("target_type", t.targetType)
	result.AddMetadata("snapshot_name", t.snapshotName)
	result.AddMetadata("project_id", t.projectID)
	result.AddMetadata("zone", t.zone)
	result.AddMetadata("retain_name", fmt.Sprintf("%t", t.config.RetainName))

	// Delete original disk if retaining name
	if t.config.RetainName {
		err := t.gcpClient.DiskClient.DeleteDisk(ctx, t.projectID, t.zone, t.diskName)
		if err != nil {
			migErr := fmt.Errorf("failed to delete original disk: %w", err)
			result.MarkFailed(migErr)
			return result, migErr
		}
		result.AddMetric("original_disk_deleted", true)
	} else {
		result.AddMetric("original_disk_deleted", false)
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
		migErr := fmt.Errorf("failed to create new disk from snapshot: %w", err)
		result.MarkFailed(migErr)
		return result, migErr
	}

	t.migrated = true
	result.AddMetric("disk_migrated", true)
	result.AddMetric("disk_size_gb", *t.disk.SizeGb)
	if t.config.Iops > 0 {
		result.AddMetric("iops", t.config.Iops)
	}
	if t.config.Throughput > 0 {
		result.AddMetric("throughput", t.config.Throughput)
	}

	result.MarkCompleted()
	return result, nil
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
		BaseTask:     NewBaseTask(id, fmt.Sprintf("%s disk %s to instance %s", strings.ToUpper(action[:1])+action[1:], diskName, instanceName), "DiskAttachment"),
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
func (t *DiskAttachmentTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("instance_name", t.instanceName)
	result.AddMetadata("disk_name", t.diskName)
	result.AddMetadata("device_name", t.deviceName)
	result.AddMetadata("action", t.action)
	result.AddMetadata("project_id", t.projectID)
	result.AddMetadata("zone", t.zone)

	switch t.action {
	case "attach":
		err := t.gcpClient.ComputeClient.AttachDisk(ctx, t.projectID, t.zone, t.instanceName, t.diskName, t.deviceName)
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
		result.AddMetric("disk_attached", true)
	case "detach":
		err := t.gcpClient.ComputeClient.DetachDisk(ctx, t.projectID, t.zone, t.instanceName, t.deviceName)
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
		result.AddMetric("disk_detached", true)
	default:
		err := fmt.Errorf("unknown disk attachment action: %s", t.action)
		result.MarkFailed(err)
		return result, err
	}

	t.executed = true
	result.MarkCompleted()
	return result, nil
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
		BaseTask:     NewBaseTask(id, fmt.Sprintf("Clean up %s %s", resourceType, resourceID), "Cleanup"),
		projectID:    projectID,
		resourceID:   resourceID,
		resourceType: resourceType,
		gcpClient:    gcpClient,
	}
}

// Execute performs the cleanup operation
func (t *CleanupTask) Execute(ctx context.Context) (*TaskResult, error) {
	result := NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("resource_type", t.resourceType)
	result.AddMetadata("resource_id", t.resourceID)
	result.AddMetadata("project_id", t.projectID)

	switch t.resourceType {
	case "snapshot":
		err := t.gcpClient.SnapshotClient.DeleteSnapshot(ctx, t.projectID, t.resourceID)
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
		result.AddMetric("snapshot_deleted", true)
	default:
		err := fmt.Errorf("unknown resource type for cleanup: %s", t.resourceType)
		result.MarkFailed(err)
		return result, err
	}

	result.MarkCompleted()
	return result, nil
}

// EnhancedCleanupTask wraps multi-level cleanup operations
type EnhancedCleanupTask struct {
	*BaseTask
	cleanupManager *migrator.MultiLevelCleanupManager
	cleanupLevel   migrator.CleanupLevel
	taskID         string
	snapshotName   string
}

// NewEnhancedCleanupTask creates a new enhanced cleanup task for task-level cleanup
func NewEnhancedCleanupTask(id string, cleanupManager *migrator.MultiLevelCleanupManager, taskID, snapshotName string) *EnhancedCleanupTask {
	return &EnhancedCleanupTask{
		BaseTask:       NewBaseTask(id, fmt.Sprintf("Clean up snapshot %s for task %s", snapshotName, taskID), "EnhancedCleanup"),
		cleanupManager: cleanupManager,
		cleanupLevel:   migrator.CleanupLevelTask,
		taskID:         taskID,
		snapshotName:   snapshotName,
	}
}

// NewSessionCleanupTask creates a new enhanced cleanup task for session-level cleanup
func NewSessionCleanupTask(id string, cleanupManager *migrator.MultiLevelCleanupManager) *EnhancedCleanupTask {
	return &EnhancedCleanupTask{
		BaseTask:       NewBaseTask(id, "Clean up all snapshots for migration session", "SessionCleanup"),
		cleanupManager: cleanupManager,
		cleanupLevel:   migrator.CleanupLevelSession,
	}
}

// NewEmergencyCleanupTask creates a new enhanced cleanup task for emergency cleanup
func NewEmergencyCleanupTask(id string, cleanupManager *migrator.MultiLevelCleanupManager) *EnhancedCleanupTask {
	return &EnhancedCleanupTask{
		BaseTask:       NewBaseTask(id, "Clean up all expired snapshots", "EmergencyCleanup"),
		cleanupManager: cleanupManager,
		cleanupLevel:   migrator.CleanupLevelEmergency,
	}
}

// Execute performs the enhanced cleanup operation
func (t *EnhancedCleanupTask) Execute(ctx context.Context) (*TaskResult, error) {
	taskResult := NewTaskResult(t.GetID(), t.GetName())
	taskResult.MarkStarted()
	taskResult.AddMetadata("cleanup_level", t.cleanupLevel.String())
	if t.taskID != "" {
		taskResult.AddMetadata("task_id", t.taskID)
	}
	if t.snapshotName != "" {
		taskResult.AddMetadata("snapshot_name", t.snapshotName)
	}

	var cleanupResult *migrator.CleanupResult
	var err error

	switch t.cleanupLevel {
	case migrator.CleanupLevelTask:
		cleanupResult = t.cleanupManager.CleanupTaskSnapshot(ctx, t.taskID, t.snapshotName)
	case migrator.CleanupLevelSession:
		cleanupResult = t.cleanupManager.CleanupSessionSnapshots(ctx)
	case migrator.CleanupLevelEmergency:
		cleanupResult = t.cleanupManager.CleanupExpiredSnapshots(ctx)
	default:
		err = fmt.Errorf("unknown cleanup level: %v", t.cleanupLevel)
		taskResult.MarkFailed(err)
		return taskResult, err
	}

	// Add cleanup metrics to task result
	taskResult.AddMetric("snapshots_found", cleanupResult.SnapshotsFound)
	taskResult.AddMetric("snapshots_deleted", cleanupResult.SnapshotsDeleted)
	taskResult.AddMetric("snapshots_failed", len(cleanupResult.SnapshotsFailed))
	taskResult.AddMetric("duration_seconds", cleanupResult.Duration.Seconds())

	// Check if cleanup had any errors
	if len(cleanupResult.Errors) > 0 {
		// Return the first error, but log all errors
		for i, cleanupErr := range cleanupResult.Errors {
			if i == 0 {
				err = cleanupErr
			}
		}
		taskResult.MarkFailed(err)
		return taskResult, err
	}

	taskResult.MarkCompleted()
	return taskResult, nil
}

// GetCleanupResult returns the result of the last cleanup operation
func (t *EnhancedCleanupTask) GetCleanupResult() *migrator.CleanupResult {
	// This would need to be stored during Execute() if we want to retrieve it later
	// For now, this is a placeholder for future enhancement
	return nil
}
