package dag

import (
	"context"
	"sync"
	"time"
)

// BaseNode provides a basic implementation of the Node interface that can be embedded
type BaseNode struct {
	task       Task
	status     NodeStatus
	err        error
	startTime  *time.Time
	endTime    *time.Time
	taskResult *TaskResult
	mutex      sync.RWMutex
}

// NewBaseNode creates a new BaseNode with the given task
func NewBaseNode(task Task) *BaseNode {
	return &BaseNode{
		task:   task,
		status: StatusPending,
	}
}

// ID returns the unique identifier for this node
func (b *BaseNode) ID() string {
	return b.task.GetID()
}

// Execute runs the node's task
func (b *BaseNode) Execute(ctx context.Context) error {
	b.mutex.Lock()
	b.status = StatusRunning
	now := time.Now()
	b.startTime = &now
	b.mutex.Unlock()
	
	taskResult, err := b.task.Execute(ctx)
	
	b.mutex.Lock()
	end := time.Now()
	b.endTime = &end
	b.taskResult = taskResult
	
	if err != nil {
		b.status = StatusFailed
		b.err = err
	} else {
		b.status = StatusCompleted
		b.err = nil
	}
	b.mutex.Unlock()
	
	return err
}

// GetTaskResult returns the TaskResult from the last execution
func (b *BaseNode) GetTaskResult() *TaskResult {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.taskResult
}

// Rollback is a no-op for the simplified Task interface
// Rollback functionality has been removed as part of the interface simplification
func (b *BaseNode) Rollback(ctx context.Context) error {
	// Rollback is not supported in the simplified Task interface
	return nil
}

// GetStatus returns the current execution status of the node
func (b *BaseNode) GetStatus() NodeStatus {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.status
}

// SetStatus updates the execution status of the node
func (b *BaseNode) SetStatus(status NodeStatus) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.status = status
}

// GetError returns any error from the last execution
func (b *BaseNode) GetError() error {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.err
}

// SetError sets an error from execution
func (b *BaseNode) SetError(err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.err = err
	if err != nil {
		b.status = StatusFailed
	}
}

// GetTask returns the underlying task
func (b *BaseNode) GetTask() Task {
	return b.task
}

// GetStartTime returns when the node started execution
func (b *BaseNode) GetStartTime() *time.Time {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.startTime
}

// GetEndTime returns when the node finished execution
func (b *BaseNode) GetEndTime() *time.Time {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.endTime
}

// SetStartTime sets the execution start time
func (b *BaseNode) SetStartTime(t *time.Time) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.startTime = t
}

// SetEndTime sets the execution end time
func (b *BaseNode) SetEndTime(t *time.Time) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.endTime = t
}