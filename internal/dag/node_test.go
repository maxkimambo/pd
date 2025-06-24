package dag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBaseNode(t *testing.T) {
	task := newMockTask("test-1", "migration", "Test migration task")
	node := NewBaseNode(task)
	
	assert.Equal(t, "test-1", node.ID())
	assert.Equal(t, StatusPending, node.GetStatus())
	assert.NoError(t, node.GetError())
	assert.Nil(t, node.GetStartTime())
	assert.Nil(t, node.GetEndTime())
	assert.Equal(t, task, node.GetTask())
}

func TestBaseNode_Execute_Success(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	node := NewBaseNode(task)
	
	ctx := context.Background()
	err := node.Execute(ctx)
	
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, node.GetStatus())
	assert.NoError(t, node.GetError())
	assert.NotNil(t, node.GetStartTime())
	assert.NotNil(t, node.GetEndTime())
	assert.True(t, node.GetEndTime().After(*node.GetStartTime()) || node.GetEndTime().Equal(*node.GetStartTime()))
}

func TestBaseNode_Execute_Failure(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	expectedErr := errors.New("execution failed")
	task.executeFunc = func(ctx context.Context) error {
		return expectedErr
	}
	
	node := NewBaseNode(task)
	ctx := context.Background()
	err := node.Execute(ctx)
	
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, StatusFailed, node.GetStatus())
	assert.Equal(t, expectedErr, node.GetError())
	assert.NotNil(t, node.GetStartTime())
	assert.NotNil(t, node.GetEndTime())
}

func TestBaseNode_Execute_WithDelay(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	task.executeFunc = func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond) // Small delay to test timing
		return nil
	}
	
	node := NewBaseNode(task)
	ctx := context.Background()
	
	startBefore := time.Now()
	err := node.Execute(ctx)
	endAfter := time.Now()
	
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, node.GetStatus())
	
	nodeStart := node.GetStartTime()
	nodeEnd := node.GetEndTime()
	require.NotNil(t, nodeStart)
	require.NotNil(t, nodeEnd)
	
	// Verify timing is reasonable
	assert.True(t, nodeStart.After(startBefore) || nodeStart.Equal(startBefore))
	assert.True(t, nodeEnd.Before(endAfter) || nodeEnd.Equal(endAfter))
	assert.True(t, nodeEnd.After(*nodeStart))
	
	duration := nodeEnd.Sub(*nodeStart)
	assert.True(t, duration >= 10*time.Millisecond)
}

func TestBaseNode_Rollback(t *testing.T) {
	tests := []struct {
		name         string
		rollbackFunc func(ctx context.Context) error
		expectError  bool
	}{
		{
			name:         "Success",
			rollbackFunc: nil, // Default returns nil
			expectError:  false,
		},
		{
			name: "Error",
			rollbackFunc: func(ctx context.Context) error {
				return errors.New("rollback failed")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := newMockTask("test-1", "test", "Test task")
			task.rollbackFunc = tt.rollbackFunc
			node := NewBaseNode(task)
			
			err := node.Rollback(context.Background())
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBaseNode_StatusTransitions(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	node := NewBaseNode(task)
	
	// Initial state
	assert.Equal(t, StatusPending, node.GetStatus())
	
	// Manual status change
	node.SetStatus(StatusCancelled)
	assert.Equal(t, StatusCancelled, node.GetStatus())
	
	// Reset to pending for execution test
	node.SetStatus(StatusPending)
	
	// Execute should change status to Running then Completed
	ctx := context.Background()
	
	// Use a task that we can control timing on
	executed := false
	task.executeFunc = func(ctx context.Context) error {
		// At this point, status should be Running
		assert.Equal(t, StatusRunning, node.GetStatus())
		executed = true
		return nil
	}
	
	err := node.Execute(ctx)
	assert.NoError(t, err)
	assert.True(t, executed)
	assert.Equal(t, StatusCompleted, node.GetStatus())
}

func TestBaseNode_ErrorHandling(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	node := NewBaseNode(task)
	
	// Initially no error
	assert.NoError(t, node.GetError())
	
	// Set error manually
	testErr := errors.New("test error")
	node.SetError(testErr)
	assert.Equal(t, testErr, node.GetError())
	assert.Equal(t, StatusFailed, node.GetStatus())
	
	// Clear error by setting nil
	node.SetError(nil)
	assert.NoError(t, node.GetError())
	assert.Equal(t, StatusFailed, node.GetStatus()) // Status doesn't auto-revert
}

func TestBaseNode_ConcurrentAccess(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	node := NewBaseNode(task)
	
	// Test concurrent reads and writes
	done := make(chan bool, 10)
	
	// Start multiple goroutines that access node state
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				_ = node.GetStatus()
				_ = node.GetError()
				_ = node.GetStartTime()
				_ = node.GetEndTime()
			}
		}()
	}
	
	// Start goroutines that modify state
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 50; j++ {
				if id%2 == 0 {
					node.SetStatus(StatusRunning)
					node.SetError(errors.New("test"))
				} else {
					node.SetStatus(StatusPending)
					node.SetError(nil)
				}
			}
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Node should still be functional
	assert.NotEmpty(t, node.ID())
	assert.NotNil(t, node.GetTask())
}

func TestBaseNode_ContextCancellation(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	node := NewBaseNode(task)
	
	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	
	task.executeFunc = func(ctx context.Context) error {
		// Simulate some work then check if cancelled
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	
	// Cancel context before execution completes
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	
	err := node.Execute(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, StatusFailed, node.GetStatus())
	assert.Equal(t, context.Canceled, node.GetError())
}

func TestNode_InterfaceCompliance(t *testing.T) {
	task := newMockTask("test", "type", "description")
	
	// Verify that BaseNode implements Node interface
	var node Node = NewBaseNode(task)
	
	assert.Equal(t, "test", node.ID())
	assert.Equal(t, StatusPending, node.GetStatus())
	assert.NoError(t, node.GetError())
	assert.Equal(t, task, node.GetTask())
	assert.Nil(t, node.GetStartTime())
	assert.Nil(t, node.GetEndTime())
	
	// Test all interface methods
	node.SetStatus(StatusRunning)
	assert.Equal(t, StatusRunning, node.GetStatus())
	
	testErr := errors.New("test")
	node.SetError(testErr)
	assert.Equal(t, testErr, node.GetError())
	
	assert.NoError(t, node.Execute(context.Background()))
	assert.NoError(t, node.Rollback(context.Background()))
}