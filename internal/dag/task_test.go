package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBaseTask(t *testing.T) {
	task := NewBaseTask("test-1", "Test migration task", "migration")

	assert.Equal(t, "test-1", task.GetID())
	assert.Equal(t, "Test migration task", task.GetName())
	assert.Equal(t, "migration", task.GetType())
}

func TestBaseTask_GetID(t *testing.T) {
	task := NewBaseTask("unique-id", "Test task", "description")
	assert.Equal(t, "unique-id", task.GetID())
}

func TestBaseTask_GetType(t *testing.T) {
	task := NewBaseTask("id", "Task name", "snapshot")
	assert.Equal(t, "snapshot", task.GetType())
}

func TestBaseTask_GetDescription(t *testing.T) {
	task := NewBaseTask("id", "Human readable description", "type")
	assert.Equal(t, "Human readable description", task.GetDescription())
}

// BaseTask is now just a helper for embedding, it doesn't implement Execute
// The actual task implementations should be tested individually

func TestMockTask_Execute(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context) (*TaskResult, error)
		expectError bool
	}{
		{
			name:        "Success",
			executeFunc: nil, // Default returns success
			expectError: false,
		},
		{
			name: "Error",
			executeFunc: func(ctx context.Context) (*TaskResult, error) {
				result := NewTaskResult("id", "type")
				result.MarkStarted()
				err := errors.New("execution failed")
				result.MarkFailed(err)
				return result, err
			},
			expectError: true,
		},
		{
			name: "Success with custom function",
			executeFunc: func(ctx context.Context) (*TaskResult, error) {
				result := NewTaskResult("id", "type")
				result.MarkStarted()
				result.MarkCompleted()
				return result, nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := newMockTask("test-1", "test", "Test task")
			task.executeFunc = tt.executeFunc

			result, err := task.Execute(context.Background())

			assert.NotNil(t, result)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, TaskStatusFailed, result.Status)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, TaskStatusCompleted, result.Status)
			}
		})
	}
}

// Rollback functionality has been removed from the simplified Task interface

func TestTask_InterfaceCompliance(t *testing.T) {
	// Verify that mockTask implements Task interface
	var task Task = newMockTask("test", "description", "type")

	assert.Equal(t, "test", task.GetID())
	assert.Equal(t, "description", task.GetName())

	// Test Execute method returns TaskResult
	result, err := task.Execute(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test", result.TaskID)
	assert.Equal(t, "description", result.TaskName)
}

func TestTask_ContextCancellation(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	task.executeFunc = func(ctx context.Context) (*TaskResult, error) {
		result := NewTaskResult("test-1", "Test task")
		result.MarkStarted()
		if ctx.Err() != nil {
			result.MarkFailed(ctx.Err())
			return result, ctx.Err()
		}
		result.MarkCompleted()
		return result, nil
	}

	result, err := task.Execute(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.NotNil(t, result)
	assert.Equal(t, TaskStatusFailed, result.Status)
}
