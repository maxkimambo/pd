package dag

import "context"

// Task represents a unit of work in the migration process
type Task interface {
	// Execute performs the task's work
	Execute(ctx context.Context) error
	
	// Rollback performs cleanup if execution fails
	Rollback(ctx context.Context) error
	
	// GetID returns the unique identifier for this task
	GetID() string
	
	// GetType returns the task type
	GetType() string
	
	// GetDescription returns a human-readable description
	GetDescription() string
	
	// Validate checks if the task can be executed
	Validate() error
}

// BaseTask provides common functionality for tasks
type BaseTask struct {
	id          string
	taskType    string
	description string
}

// NewBaseTask creates a new base task
func NewBaseTask(id, taskType, description string) *BaseTask {
	return &BaseTask{
		id:          id,
		taskType:    taskType,
		description: description,
	}
}

// GetID returns the unique identifier for this task
func (t *BaseTask) GetID() string {
	return t.id
}

// GetType returns the task type
func (t *BaseTask) GetType() string {
	return t.taskType
}

// GetDescription returns a human-readable description
func (t *BaseTask) GetDescription() string {
	return t.description
}

// Validate checks if the task can be executed (default implementation)
func (t *BaseTask) Validate() error {
	return nil
}

// Execute must be implemented by concrete task types
func (t *BaseTask) Execute(ctx context.Context) error {
	panic("Execute method must be implemented by concrete task types")
}

// Rollback must be implemented by concrete task types
func (t *BaseTask) Rollback(ctx context.Context) error {
	panic("Rollback method must be implemented by concrete task types")
}