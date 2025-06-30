package taskmanager

import "context"

// TaskFunc is the function signature for a task's execution logic.
type TaskFunc func(ctx context.Context, sharedCtx *SharedContext) error

// Task represents a single unit of work in a workflow.
type Task struct {
	ID        string
	Handler   TaskFunc
	DependsOn []string // IDs of tasks this task depends on
}
