package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBaseTask(t *testing.T) {
	task := NewBaseTask("test-1", "migration", "Test migration task")
	
	assert.Equal(t, "test-1", task.GetID())
	assert.Equal(t, "migration", task.GetType())
	assert.Equal(t, "Test migration task", task.GetDescription())
}

func TestBaseTask_GetID(t *testing.T) {
	task := NewBaseTask("unique-id", "test", "description")
	assert.Equal(t, "unique-id", task.GetID())
}

func TestBaseTask_GetType(t *testing.T) {
	task := NewBaseTask("id", "snapshot", "description")
	assert.Equal(t, "snapshot", task.GetType())
}

func TestBaseTask_GetDescription(t *testing.T) {
	task := NewBaseTask("id", "type", "Human readable description")
	assert.Equal(t, "Human readable description", task.GetDescription())
}

func TestBaseTask_Validate(t *testing.T) {
	task := NewBaseTask("id", "type", "description")
	err := task.Validate()
	assert.NoError(t, err)
}

func TestBaseTask_Execute_Panics(t *testing.T) {
	task := NewBaseTask("id", "type", "description")
	
	assert.Panics(t, func() {
		task.Execute(context.Background())
	})
}

func TestBaseTask_Rollback_Panics(t *testing.T) {
	task := NewBaseTask("id", "type", "description")
	
	assert.Panics(t, func() {
		task.Rollback(context.Background())
	})
}

func TestMockTask_Execute(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context) error
		expectError bool
	}{
		{
			name:        "Success",
			executeFunc: nil, // Default returns nil
			expectError: false,
		},
		{
			name: "Error",
			executeFunc: func(ctx context.Context) error {
				return errors.New("execution failed")
			},
			expectError: true,
		},
		{
			name: "Success with custom function",
			executeFunc: func(ctx context.Context) error {
				return nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := newMockTask("test-1", "test", "Test task")
			task.executeFunc = tt.executeFunc
			
			err := task.Execute(context.Background())
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMockTask_Rollback(t *testing.T) {
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
		{
			name: "Success with custom function",
			rollbackFunc: func(ctx context.Context) error {
				return nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := newMockTask("test-1", "test", "Test task")
			task.rollbackFunc = tt.rollbackFunc
			
			err := task.Rollback(context.Background())
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTask_InterfaceCompliance(t *testing.T) {
	// Verify that mockTask implements Task interface
	var task Task = newMockTask("test", "type", "description")
	
	assert.Equal(t, "test", task.GetID())
	assert.Equal(t, "type", task.GetType())
	assert.Equal(t, "description", task.GetDescription())
	assert.NoError(t, task.Validate())
	assert.NoError(t, task.Execute(context.Background()))
	assert.NoError(t, task.Rollback(context.Background()))
}

func TestTask_ContextCancellation(t *testing.T) {
	task := newMockTask("test-1", "test", "Test task")
	
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	task.executeFunc = func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
	
	err := task.Execute(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}