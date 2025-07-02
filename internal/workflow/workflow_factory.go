package workflow

import (
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/migrator/tasks"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// WorkflowFactory creates workflows for different migration scenarios
type WorkflowFactory struct {
	gcpClient *gcp.Clients
	config    *migrator.Config
}

// NewWorkflowFactory creates a new WorkflowFactory instance
func NewWorkflowFactory(gcpClient *gcp.Clients, config *migrator.Config) *WorkflowFactory {
	return &WorkflowFactory{
		gcpClient: gcpClient,
		config:    config,
	}
}

// CreateComputeDiskMigrationWorkflow creates a workflow for migrating disks attached to a compute instance
func (f *WorkflowFactory) CreateComputeDiskMigrationWorkflow(workflowID string, instance *computepb.Instance) (*taskmanager.Workflow, error) {
	builder := taskmanager.NewWorkflowBuilder(workflowID)

	// Create task instances
	validateTask := tasks.NewValidateInstanceTask(instance, f.config.TargetDiskType, f.config, f.gcpClient)
	checkStateTask := tasks.NewCheckInstanceStateTask()
	snapshotTask := createSnapshotTask(f.config)
	stopTask := tasks.NewStopInstanceTask()
	migrateTask := createMigrateTask(f.config) // Use the wrapper that calls MigrateInstanceNonBootDisks
	startTask := tasks.NewStartInstanceTask()
	verifyTask := tasks.NewVerifyMigrationTask()
	cleanupTask := tasks.NewCleanupSnapshotsTask()

	// Add tasks to workflow
	builder.
		AddTask("validate_instance", validateTask).
		AddTask("check_instance_state", checkStateTask).
		AddTask("snapshot_disks", snapshotTask).
		AddTask("stop_instance", stopTask).
		AddTask("migrate_disks", migrateTask). // This handles detach, migrate, and reattach
		AddTask("start_instance", startTask).
		AddTask("verify_migration", verifyTask).
		AddTask("cleanup_snapshots", cleanupTask)

	// Define dependencies
	builder.
		AddDependency("check_instance_state", "validate_instance").
		AddDependency("snapshot_disks", "check_instance_state").
		AddDependency("stop_instance", "snapshot_disks").
		AddDependency("migrate_disks", "stop_instance").
		AddDependency("start_instance", "migrate_disks").
		AddDependency("verify_migration", "start_instance").
		AddDependency("cleanup_snapshots", "verify_migration")

	return builder.Build()
}

// CreateDetachedDiskMigrationWorkflow creates a workflow for migrating detached disks
func (f *WorkflowFactory) CreateDetachedDiskMigrationWorkflow(workflowID string, disks []*computepb.Disk) (*taskmanager.Workflow, error) {
	// For now, return an error as this is not yet implemented
	return nil, fmt.Errorf("detached disk migration workflow not yet implemented")
}

// CreateBatchInstanceWorkflow creates a workflow for migrating multiple instances
func (f *WorkflowFactory) CreateBatchInstanceWorkflow(workflowID string, instances []*computepb.Instance) (*taskmanager.Workflow, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances provided for batch workflow")
	}

	builder := taskmanager.NewWorkflowBuilder(workflowID)

	var previousInstanceTask string

	for i, instance := range instances {
		if instance == nil || instance.Name == nil {
			continue
		}

		instancePrefix := fmt.Sprintf("instance_%d_%s", i, *instance.Name)

		// Create task instances for this instance
		validateTask := tasks.NewValidateInstanceTask(instance, f.config.TargetDiskType, f.config, f.gcpClient)
		checkStateTask := tasks.NewCheckInstanceStateTask()
		snapshotTask := createSnapshotTask(f.config)
		stopTask := tasks.NewStopInstanceTask()
		migrateTask := createMigrateTask(f.config)
		startTask := tasks.NewStartInstanceTask()
		verifyTask := tasks.NewVerifyMigrationTask()
		cleanupTask := tasks.NewCleanupSnapshotsTask()

		// Add tasks with unique IDs
		builder.
			AddTask(fmt.Sprintf("%s_validate", instancePrefix), validateTask).
			AddTask(fmt.Sprintf("%s_check_state", instancePrefix), checkStateTask).
			AddTask(fmt.Sprintf("%s_snapshot", instancePrefix), snapshotTask).
			AddTask(fmt.Sprintf("%s_stop", instancePrefix), stopTask).
			AddTask(fmt.Sprintf("%s_migrate", instancePrefix), migrateTask).
			AddTask(fmt.Sprintf("%s_start", instancePrefix), startTask).
			AddTask(fmt.Sprintf("%s_verify", instancePrefix), verifyTask).
			AddTask(fmt.Sprintf("%s_cleanup", instancePrefix), cleanupTask)

		// Define dependencies within this instance's tasks
		builder.
			AddDependency(fmt.Sprintf("%s_check_state", instancePrefix), fmt.Sprintf("%s_validate", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_snapshot", instancePrefix), fmt.Sprintf("%s_check_state", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_stop", instancePrefix), fmt.Sprintf("%s_snapshot", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_migrate", instancePrefix), fmt.Sprintf("%s_stop", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_start", instancePrefix), fmt.Sprintf("%s_migrate", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_verify", instancePrefix), fmt.Sprintf("%s_start", instancePrefix)).
			AddDependency(fmt.Sprintf("%s_cleanup", instancePrefix), fmt.Sprintf("%s_verify", instancePrefix))

		// Chain instances sequentially (for now)
		if previousInstanceTask != "" {
			builder.AddDependency(fmt.Sprintf("%s_validate", instancePrefix), previousInstanceTask)
		}

		previousInstanceTask = fmt.Sprintf("%s_cleanup", instancePrefix)
	}

	return builder.Build()
}
