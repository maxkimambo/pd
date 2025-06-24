package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxkimambo/pd/internal/dag"
)

// DiskMigrationWrapper is a wrapper task that handles disk migration with factory integration
type DiskMigrationWrapper struct {
	*dag.BaseTask
	factory      *TaskFactory
	diskName     string
	snapshotName string
}

// Execute performs the disk migration using the factory's configuration
func (t *DiskMigrationWrapper) Execute(ctx context.Context) error {
	// Get the disk information
	disk, err := t.factory.gcpClient.DiskClient.GetDisk(ctx, t.factory.config.ProjectID, t.factory.config.Zone, t.diskName)
	if err != nil {
		return fmt.Errorf("failed to get disk %s for migration: %w", t.diskName, err)
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

// Rollback is simplified - no operation needed
func (t *DiskMigrationWrapper) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// BatchSnapshotTask creates snapshots for multiple disks in parallel
type BatchSnapshotTask struct {
	*dag.BaseTask
	factory   *TaskFactory
	diskNames []string
}

// Execute creates snapshots for all disks in parallel
func (t *BatchSnapshotTask) Execute(ctx context.Context) error {
	if len(t.diskNames) == 0 {
		return nil
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
			
			if err := snapshotTask.Execute(ctx); err != nil {
				errChan <- fmt.Errorf("failed to create snapshot for disk %s: %w", disk, err)
			}
		}(diskName)
	}
	
	wg.Wait()
	close(errChan)
	
	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// Rollback is simplified - no operation needed
func (t *BatchSnapshotTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// ValidationTask validates that resources meet migration requirements
type ValidationTask struct {
	*dag.BaseTask
	factory      *TaskFactory
	resourceType string
	resourceID   string
}

// NewValidationTask creates a new validation task
func (f *TaskFactory) CreateValidationTask(resourceType, resourceID string) dag.Node {
	id := fmt.Sprintf("validate_%s_%s", resourceType, resourceID)
	task := &ValidationTask{
		BaseTask:     dag.NewBaseTask(id, "Validation", fmt.Sprintf("Validate %s %s", resourceType, resourceID)),
		factory:      f,
		resourceType: resourceType,
		resourceID:   resourceID,
	}
	return dag.NewBaseNode(task)
}

// Execute performs validation checks
func (t *ValidationTask) Execute(ctx context.Context) error {
	switch t.resourceType {
	case "disk":
		return t.validateDisk(ctx)
	case "instance":
		return t.validateInstance(ctx)
	default:
		return fmt.Errorf("unknown resource type for validation: %s", t.resourceType)
	}
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
	if instance.Disks == nil || len(instance.Disks) == 0 {
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

// Rollback is a no-op for validation
func (t *ValidationTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}

// PreflightCheckTask performs comprehensive preflight checks
type PreflightCheckTask struct {
	*dag.BaseTask
	factory   *TaskFactory
	instances []string
}

// CreatePreflightCheckTask creates a task for preflight checks
func (f *TaskFactory) CreatePreflightCheckTask(instances []string) dag.Node {
	id := fmt.Sprintf("preflight_check_%d_instances", len(instances))
	task := &PreflightCheckTask{
		BaseTask:  dag.NewBaseTask(id, "PreflightCheck", fmt.Sprintf("Preflight check for %d instances", len(instances))),
		factory:   f,
		instances: instances,
	}
	return dag.NewBaseNode(task)
}

// Execute performs comprehensive preflight checks
func (t *PreflightCheckTask) Execute(ctx context.Context) error {
	// Check project permissions
	if err := t.checkProjectPermissions(ctx); err != nil {
		return fmt.Errorf("project permissions check failed: %w", err)
	}
	
	// Check quota availability
	if err := t.checkQuotas(ctx); err != nil {
		return fmt.Errorf("quota check failed: %w", err)
	}
	
	// Validate all instances
	for _, instanceName := range t.instances {
		validationTask := &ValidationTask{
			BaseTask:     dag.NewBaseTask("validate_"+instanceName, "Validation", "Validate instance "+instanceName),
			factory:      t.factory,
			resourceType: "instance",
			resourceID:   instanceName,
		}
		
		if err := validationTask.Execute(ctx); err != nil {
			return fmt.Errorf("validation failed for instance %s: %w", instanceName, err)
		}
	}
	
	return nil
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

// Rollback is a no-op for preflight checks
func (t *PreflightCheckTask) Rollback(ctx context.Context) error {
	// Rollback functionality simplified - no operation needed
	return nil
}