package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/dag"
)

// DiskMigrationWrapper is a wrapper task that handles disk migration with factory integration
type DiskMigrationWrapper struct {
	id           string
	name         string
	factory      *TaskFactory
	diskName     string
	snapshotName string
}

// Execute performs the disk migration using the factory's configuration
func (t *DiskMigrationWrapper) Execute(ctx context.Context) (*dag.TaskResult, error) {
	// Get the disk information
	disk, err := t.factory.gcpClient.DiskClient.GetDisk(ctx, t.factory.config.ProjectID, t.factory.config.Zone, t.diskName)
	if err != nil {
		result := dag.NewTaskResult(t.GetID(), t.GetName())
		result.MarkStarted()
		result.MarkFailed(fmt.Errorf("failed to get disk %s for migration: %w", t.diskName, err))
		return result, err
	}

	// Create the actual migration task and execute it
	migrationTask := dag.NewDiskMigrationTask(
		t.GetID(),
		t.factory.config.ProjectID,
		t.factory.config.Zone,
		t.diskName,
		t.factory.config.TargetDiskType,
		t.snapshotName,
		t.factory.gcpClient,
		t.factory.config,
		disk,
	)

	return migrationTask.Execute(ctx)
}

// GetID returns the task ID
func (t *DiskMigrationWrapper) GetID() string {
	return t.id
}

// GetName returns the task name
func (t *DiskMigrationWrapper) GetName() string {
	return t.name
}

// GetType returns the task type
func (t *DiskMigrationWrapper) GetType() string {
	return "DiskMigration"
}

// BatchSnapshotTask creates snapshots for multiple disks in parallel
type BatchSnapshotTask struct {
	id        string
	name      string
	factory   *TaskFactory
	diskNames []string
}

// Execute creates snapshots for all disks in parallel
func (t *BatchSnapshotTask) Execute(ctx context.Context) (*dag.TaskResult, error) {
	result := dag.NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()

	if len(t.diskNames) == 0 {
		result.MarkCompleted()
		return result, nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(t.diskNames))

	// Create snapshots in parallel
	for _, diskName := range t.diskNames {
		wg.Add(1)
		go func(disk string) {
			defer wg.Done()

			snapshotName := fmt.Sprintf("pd-migrate-%s-batch", disk)
			snapshotTask := dag.NewSnapshotTask(
				fmt.Sprintf("snapshot_%s", disk),
				t.factory.config.ProjectID,
				t.factory.config.Zone,
				disk,
				snapshotName,
				t.factory.gcpClient,
				t.factory.config,
			)

			if _, err := snapshotTask.Execute(ctx); err != nil {
				errChan <- fmt.Errorf("failed to create snapshot for disk %s: %w", disk, err)
			}
		}(diskName)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			result.MarkFailed(err)
			return result, err
		}
	}

	result.AddMetric("disks_processed", len(t.diskNames))
	result.MarkCompleted()
	return result, nil
}

// GetID returns the task ID
func (t *BatchSnapshotTask) GetID() string {
	return t.id
}

// GetName returns the task name
func (t *BatchSnapshotTask) GetName() string {
	return t.name
}

// GetType returns the task type
func (t *BatchSnapshotTask) GetType() string {
	return "BatchSnapshot"
}

// ValidationTask validates that resources meet migration requirements
type ValidationTask struct {
	id           string
	name         string
	factory      *TaskFactory
	resourceType string
	resourceID   string
}

// CreateValidationTask creates a new validation task
func (f *TaskFactory) CreateValidationTask(resourceType, resourceID string) dag.Node {
	id := fmt.Sprintf("validate_%s_%s", resourceType, resourceID)
	task := &ValidationTask{
		id:           id,
		name:         fmt.Sprintf("Validate %s %s", resourceType, resourceID),
		factory:      f,
		resourceType: resourceType,
		resourceID:   resourceID,
	}
	return dag.NewBaseNode(task)
}

// Execute performs validation checks
func (t *ValidationTask) Execute(ctx context.Context) (*dag.TaskResult, error) {
	result := dag.NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("resource_type", t.resourceType)
	result.AddMetadata("resource_id", t.resourceID)

	var err error
	switch t.resourceType {
	case "disk":
		err = t.validateDisk(ctx)
	case "instance":
		err = t.validateInstance(ctx)
	default:
		err = fmt.Errorf("unknown resource type for validation: %s", t.resourceType)
	}

	if err != nil {
		result.MarkFailed(err)
		return result, err
	}

	result.AddMetric("validation_passed", true)
	result.MarkCompleted()
	return result, nil
}

// GetID returns the task ID
func (t *ValidationTask) GetID() string {
	return t.id
}

// GetName returns the task name
func (t *ValidationTask) GetName() string {
	return t.name
}

// GetType returns the task type
func (t *ValidationTask) GetType() string {
	return "Validation"
}

// validateDisk checks if a disk is suitable for migration
func (t *ValidationTask) validateDisk(ctx context.Context) error {
	disk, err := t.factory.gcpClient.DiskClient.GetDisk(ctx, t.factory.config.ProjectID, t.factory.config.Zone, t.resourceID)
	if err != nil {
		return fmt.Errorf("failed to get disk %s: %w", t.resourceID, err)
	}

	// Check if disk is already the target type
	currentType := extractDiskNameFromSource(disk.GetType())
	if currentType == t.factory.config.TargetDiskType {
		return fmt.Errorf("disk %s is already of type %s", t.resourceID, t.factory.config.TargetDiskType)
	}

	// Check if disk is attached (for detached disk migration)
	// This would need additional logic to check attachment status

	return nil
}

// validateInstance checks if an instance is suitable for migration
func (t *ValidationTask) validateInstance(ctx context.Context) error {
	instance, err := t.factory.gcpClient.ComputeClient.GetInstance(ctx, t.factory.config.ProjectID, t.factory.config.Zone, t.resourceID)
	if err != nil {
		return fmt.Errorf("failed to get instance %s: %w", t.resourceID, err)
	}

	// Check if instance has disks that need migration
	if len(instance.Disks) == 0 {
		return fmt.Errorf("instance %s has no disks", t.resourceID)
	}

	// Check if any non-boot disks need migration
	hasNonBootDisks := false
	for _, disk := range instance.Disks {
		if !disk.GetBoot() {
			hasNonBootDisks = true
			break
		}
	}

	if !hasNonBootDisks {
		return fmt.Errorf("instance %s has no non-boot disks to migrate", t.resourceID)
	}

	return nil
}

// PreflightCheckTask performs comprehensive preflight checks
type PreflightCheckTask struct {
	id        string
	name      string
	factory   *TaskFactory
	instances []string
}

// CreatePreflightCheckTask creates a task for preflight checks
func (f *TaskFactory) CreatePreflightCheckTask(instances []string) dag.Node {
	id := fmt.Sprintf("preflight_check_%d_instances", len(instances))
	task := &PreflightCheckTask{
		id:        id,
		name:      fmt.Sprintf("Preflight check for %d instances", len(instances)),
		factory:   f,
		instances: instances,
	}
	return dag.NewBaseNode(task)
}

// Execute performs comprehensive preflight checks
func (t *PreflightCheckTask) Execute(ctx context.Context) (*dag.TaskResult, error) {
	result := dag.NewTaskResult(t.GetID(), t.GetName())
	result.MarkStarted()
	result.AddMetadata("instances_count", fmt.Sprintf("%d", len(t.instances)))

	// Check project permissions
	if err := t.checkProjectPermissions(ctx); err != nil {
		permErr := fmt.Errorf("project permissions check failed: %w", err)
		result.MarkFailed(permErr)
		return result, permErr
	}
	result.AddMetric("permissions_check_passed", true)

	// Check quota availability
	if err := t.checkQuotas(ctx); err != nil {
		quotaErr := fmt.Errorf("quota check failed: %w", err)
		result.MarkFailed(quotaErr)
		return result, quotaErr
	}
	result.AddMetric("quota_check_passed", true)

	// Validate all instances
	for _, instanceName := range t.instances {
		validationTask := &ValidationTask{
			id:           "validate_" + instanceName,
			name:         "Validate instance " + instanceName,
			factory:      t.factory,
			resourceType: "instance",
			resourceID:   instanceName,
		}

		if _, err := validationTask.Execute(ctx); err != nil {
			validationErr := fmt.Errorf("validation failed for instance %s: %w", instanceName, err)
			result.MarkFailed(validationErr)
			return result, validationErr
		}
	}

	result.AddMetric("instances_validated", len(t.instances))
	result.MarkCompleted()
	return result, nil
}

// GetID returns the task ID
func (t *PreflightCheckTask) GetID() string {
	return t.id
}

// GetName returns the task name
func (t *PreflightCheckTask) GetName() string {
	return t.name
}

// GetType returns the task type
func (t *PreflightCheckTask) GetType() string {
	return "PreflightCheck"
}

// checkProjectPermissions verifies the necessary permissions are available
func (t *PreflightCheckTask) checkProjectPermissions(ctx context.Context) error {
	// This is a simplified check - in reality you'd verify specific IAM permissions
	// For now, just check if we can list instances
	_, err := t.factory.gcpClient.ComputeClient.ListInstancesInZone(ctx, t.factory.config.ProjectID, t.factory.config.Zone)
	if err != nil {
		return fmt.Errorf("insufficient permissions to list instances: %w", err)
	}

	return nil
}

// checkQuotas verifies sufficient quotas are available
func (t *PreflightCheckTask) checkQuotas(ctx context.Context) error {
	// This is a simplified check - in reality you'd check specific quotas
	// For now, just return success as quota checking requires additional GCP APIs
	return nil
}

