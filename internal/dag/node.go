package dag

import (
	"context"
	"time"
)

// Node represents a single node in the DAG that can be executed
type Node interface {
	// ID returns the unique identifier for this node
	ID() string
	
	// Execute runs the node's task with the given context
	Execute(ctx context.Context) error
	
	// Rollback performs cleanup if execution fails
	Rollback(ctx context.Context) error
	
	// GetStatus returns the current execution status of the node
	GetStatus() NodeStatus
	
	// SetStatus updates the execution status of the node
	SetStatus(status NodeStatus)
	
	// GetError returns any error from the last execution
	GetError() error
	
	// SetError sets an error from execution
	SetError(err error)
	
	// GetTask returns the underlying task
	GetTask() Task
	
	// GetStartTime returns when the node started execution
	GetStartTime() *time.Time
	
	// GetEndTime returns when the node finished execution
	GetEndTime() *time.Time
}

// NodeStatus represents the execution status of a node
type NodeStatus int

const (
	// StatusPending indicates the node is waiting to be executed
	StatusPending NodeStatus = iota
	// StatusRunning indicates the node is currently being executed
	StatusRunning
	// StatusCompleted indicates the node has completed successfully
	StatusCompleted
	// StatusFailed indicates the node failed during execution
	StatusFailed
	// StatusCancelled indicates the node was cancelled
	StatusCancelled
)

// String returns a string representation of the NodeStatus
func (s NodeStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}