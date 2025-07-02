package tasks

import (
	"context"

	"github.com/maxkimambo/pd/internal/taskmanager"
)

// SnapshotDisksTask creates snapshots for all non-boot disks
type SnapshotDisksTask struct {
	BaseTask
}

// NewSnapshotDisksTask creates a new SnapshotDisksTask
func NewSnapshotDisksTask() taskmanager.TaskFunc {
	task := &SnapshotDisksTask{
		BaseTask: BaseTask{Name: "snapshot_disks"},
	}
	return task.Execute
}

// Execute creates snapshots for all non-boot disks attached to the instance
func (t *SnapshotDisksTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// This is a placeholder task
		// The actual snapshot creation is handled by the workflow wrapper
		// which calls migrator.SnapshotInstanceDisks
		return nil
	})
}
