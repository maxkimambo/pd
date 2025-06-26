package dag

import (
	"context"
	"time"
)

// TaskStatus represents the execution status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusSkipped   TaskStatus = "skipped"
)

// TaskResult contains execution results and metrics for a task
type TaskResult struct {
	// Task identification
	TaskID   string `json:"task_id"`
	TaskName string `json:"task_name"`

	// Execution status and timing
	Status    TaskStatus    `json:"status"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`

	// Error information
	Error        error  `json:"-"` // Not JSON serialized due to interface complexity
	ErrorMessage string `json:"error_message,omitempty"`

	// Metrics and metadata
	Metrics  map[string]interface{} `json:"metrics,omitempty"`
	Metadata map[string]string      `json:"metadata,omitempty"`

	// Resource usage (can be extended)
	ResourcesUsed map[string]interface{} `json:"resources_used,omitempty"`
}

// NewTaskResult creates a new TaskResult with basic information
func NewTaskResult(taskID, taskName string) *TaskResult {
	return &TaskResult{
		TaskID:        taskID,
		TaskName:      taskName,
		Status:        TaskStatusPending,
		Metrics:       make(map[string]interface{}),
		Metadata:      make(map[string]string),
		ResourcesUsed: make(map[string]interface{}),
	}
}

// SetError sets the error and updates the status
func (tr *TaskResult) SetError(err error) {
	tr.Error = err
	if err != nil {
		tr.ErrorMessage = err.Error()
		tr.Status = TaskStatusFailed
	}
}

// MarkStarted marks the task as started and records start time
func (tr *TaskResult) MarkStarted() {
	tr.Status = TaskStatusRunning
	tr.StartTime = time.Now()
}

// MarkCompleted marks the task as completed and calculates duration
func (tr *TaskResult) MarkCompleted() {
	tr.Status = TaskStatusCompleted
	tr.EndTime = time.Now()
	if !tr.StartTime.IsZero() {
		tr.Duration = tr.EndTime.Sub(tr.StartTime)
	}
}

func (tr *TaskResult) GetDuration() time.Duration {
	return tr.Duration
}

// MarkFailed marks the task as failed and calculates duration
func (tr *TaskResult) MarkFailed(err error) {
	tr.SetError(err)
	tr.EndTime = time.Now()
	if !tr.StartTime.IsZero() {
		tr.Duration = tr.EndTime.Sub(tr.StartTime)
	}
}

// MarkCancelled marks the task as cancelled
func (tr *TaskResult) MarkCancelled() {
	tr.Status = TaskStatusCancelled
	tr.EndTime = time.Now()
	if !tr.StartTime.IsZero() {
		tr.Duration = tr.EndTime.Sub(tr.StartTime)
	}
}

// AddMetric adds a metric value
func (tr *TaskResult) AddMetric(key string, value interface{}) {
	tr.Metrics[key] = value
}

// AddMetadata adds metadata
func (tr *TaskResult) AddMetadata(key, value string) {
	tr.Metadata[key] = value
}

// Task represents a unified interface for all task types in the DAG system.
// This simplified interface reduces complexity while maintaining essential functionality.
type Task interface {
	// Execute performs the task operation and returns TaskResult with metrics and error information
	Execute(ctx context.Context) (*TaskResult, error)

	// GetID returns the unique identifier for this task
	GetID() string

	// GetName returns a human-readable name for this task
	GetName() string
}

// TaskTypeProvider is an optional interface that tasks can implement to provide type information
// This helps with backward compatibility and logging
type TaskTypeProvider interface {
	GetType() string
}

// TaskDescriptionProvider is an optional interface that tasks can implement to provide description
// This helps with backward compatibility and logging
type TaskDescriptionProvider interface {
	GetDescription() string
}

// GetTaskType returns the task type if the task implements TaskTypeProvider, otherwise returns the task name
func GetTaskType(task Task) string {
	if typeProvider, ok := task.(TaskTypeProvider); ok {
		return typeProvider.GetType()
	}
	return task.GetName()
}

// GetTaskDescription returns the task description if the task implements TaskDescriptionProvider, otherwise returns the task name
func GetTaskDescription(task Task) string {
	if descProvider, ok := task.(TaskDescriptionProvider); ok {
		return descProvider.GetDescription()
	}
	return task.GetName()
}

// BaseTask provides common functionality for tasks implementing the new Task interface
type BaseTask struct {
	id          string
	name        string
	taskType    string
	description string
}

// NewBaseTask creates a new base task
func NewBaseTask(id, name, taskType string) *BaseTask {
	return &BaseTask{
		id:       id,
		name:     name,
		taskType: taskType,
	}
}

// NewBaseTaskWithDescription creates a new base task with description
func NewBaseTaskWithDescription(id, name, taskType, description string) *BaseTask {
	return &BaseTask{
		id:          id,
		name:        name,
		taskType:    taskType,
		description: description,
	}
}

// GetID returns the unique identifier for this task
func (t *BaseTask) GetID() string {
	return t.id
}

// GetName returns a human-readable name for this task
func (t *BaseTask) GetName() string {
	return t.name
}

// GetType returns the task type (implements TaskTypeProvider for backward compatibility)
func (t *BaseTask) GetType() string {
	return t.taskType
}

// GetDescription returns a human-readable description (implements TaskDescriptionProvider for backward compatibility)
func (t *BaseTask) GetDescription() string {
	if t.description != "" {
		return t.description
	}
	return t.name
}

// Execute must be implemented by concrete task types
func (t *BaseTask) Execute(ctx context.Context) (*TaskResult, error) {
	panic("Execute method must be implemented by concrete task types")
}
