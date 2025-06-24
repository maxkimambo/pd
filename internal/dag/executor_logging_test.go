package dag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor_ProgressLogging(t *testing.T) {
	dag := NewDAG()
	
	// Create a simple linear DAG
	task1 := &MockTask{id: "task1", taskType: "TestTask", description: "First task"}
	task2 := &MockTask{id: "task2", taskType: "TestTask", description: "Second task"}
	
	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddDependency("task1", "task2")
	require.NoError(t, err)
	
	// Create executor
	config := &ExecutorConfig{
		MaxParallelTasks: 2,
		TaskTimeout:      5 * time.Second,
		PollInterval:     10 * time.Millisecond,
	}
	executor := NewExecutor(dag, config)
	
	// Test progress tracking (before initialization results might not be ready)
	completed, total := executor.GetProgress()
	assert.Equal(t, 0, completed)
	// Total might be 0 before results are initialized, so we'll check after execution
	
	// Execute the DAG
	ctx := context.Background()
	result, err := executor.Execute(ctx)
	
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	
	// Verify final progress
	completed, total = executor.GetProgress()
	assert.Equal(t, 2, completed)
	assert.Equal(t, 2, total)
}

func TestExecutor_GetPendingDependencies(t *testing.T) {
	dag := NewDAG()
	
	// Create executor
	config := DefaultExecutorConfig()
	executor := NewExecutor(dag, config)
	
	// Initialize some mock results
	executor.results = map[string]*NodeResult{
		"completed": {NodeID: "completed", Success: true, Error: nil},
		"failed":    {NodeID: "failed", Success: false, Error: assert.AnError},
		"pending":   {NodeID: "pending", Success: false, Error: nil},
	}
	
	deps := []string{"completed", "failed", "pending"}
	pending := executor.getPendingDependencies(deps)
	
	// Only "pending" should be in the pending list
	assert.Equal(t, []string{"pending"}, pending)
}

func TestExecutor_AreDependenciesCompleted(t *testing.T) {
	dag := NewDAG()
	
	// Create executor
	config := DefaultExecutorConfig()
	executor := NewExecutor(dag, config)
	
	// Initialize some mock results
	executor.results = map[string]*NodeResult{
		"completed1": {NodeID: "completed1", Success: true, Error: nil},
		"completed2": {NodeID: "completed2", Success: true, Error: nil},
		"failed":     {NodeID: "failed", Success: false, Error: assert.AnError},
		"pending":    {NodeID: "pending", Success: false, Error: nil},
	}
	
	// Test all completed dependencies
	completedDeps := []string{"completed1", "completed2"}
	assert.True(t, executor.areDependenciesCompleted(completedDeps))
	
	// Test with failed dependency
	mixedDeps := []string{"completed1", "failed"}
	assert.False(t, executor.areDependenciesCompleted(mixedDeps))
	
	// Test with pending dependency
	pendingDeps := []string{"completed1", "pending"}
	assert.False(t, executor.areDependenciesCompleted(pendingDeps))
	
	// Test empty dependencies
	emptyDeps := []string{}
	assert.True(t, executor.areDependenciesCompleted(emptyDeps))
}