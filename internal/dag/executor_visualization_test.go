package dag

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor_VisualizationIntegration(t *testing.T) {
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
	
	// Test that visualization is available
	viz := executor.GetVisualization()
	assert.NotNil(t, viz)
	assert.Equal(t, dag, viz.dag)
}

func TestExecutor_EnableVisualization(t *testing.T) {
	dag := NewDAG()
	
	// Create a simple task that completes quickly
	task := &MockTask{
		id:          "quick-task",
		taskType:    "QuickTask",
		description: "Task that completes quickly",
		executeFunc: func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond) // Short delay
			return nil
		},
	}
	
	node := NewBaseNode(task)
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Create executor
	config := &ExecutorConfig{
		MaxParallelTasks:  1,
		TaskTimeout:       5 * time.Second,
		PollInterval:      10 * time.Millisecond,
	}
	executor := NewExecutor(dag, config)
	
	// Create temporary files for different formats
	jsonFile, err := os.CreateTemp("", "test_viz_*.json")
	require.NoError(t, err)
	defer os.Remove(jsonFile.Name())
	jsonFile.Close()
	
	dotFile, err := os.CreateTemp("", "test_viz_*.dot")
	require.NoError(t, err)
	defer os.Remove(dotFile.Name())
	dotFile.Close()
	
	txtFile, err := os.CreateTemp("", "test_viz_*.txt")
	require.NoError(t, err)
	defer os.Remove(txtFile.Name())
	txtFile.Close()
	
	// Test JSON visualization
	executor.EnableVisualization(jsonFile.Name(), 20*time.Millisecond)
	
	ctx := context.Background()
	result, err := executor.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, result.Success)
	
	// Give some time for final visualization update
	time.Sleep(100 * time.Millisecond)
	
	// Check that JSON file was created and contains data
	jsonData, err := os.ReadFile(jsonFile.Name())
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)
	assert.Contains(t, string(jsonData), "quick-task")
	
	// Test DOT visualization with a new executor
	dag2 := NewDAG()
	task2 := &MockTask{
		id:          "dot-task",
		taskType:    "DotTask",
		description: "Task for DOT visualization",
		executeFunc: func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}
	node2 := NewBaseNode(task2)
	err = dag2.AddNode(node2)
	require.NoError(t, err)
	
	executor2 := NewExecutor(dag2, config)
	executor2.EnableVisualization(dotFile.Name(), 20*time.Millisecond)
	
	result2, err := executor2.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, result2.Success)
	
	// Give some time for final visualization update
	time.Sleep(100 * time.Millisecond)
	
	// Check that DOT file was created and contains data
	dotData, err := os.ReadFile(dotFile.Name())
	assert.NoError(t, err)
	assert.NotEmpty(t, dotData)
	assert.Contains(t, string(dotData), "digraph MigrationDAG")
	assert.Contains(t, string(dotData), "dot-task")
	
	// Test text visualization with a new executor
	dag3 := NewDAG()
	task3 := &MockTask{
		id:          "text-task",
		taskType:    "TextTask",
		description: "Task for text visualization",
		executeFunc: func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}
	node3 := NewBaseNode(task3)
	err = dag3.AddNode(node3)
	require.NoError(t, err)
	
	executor3 := NewExecutor(dag3, config)
	executor3.EnableVisualization(txtFile.Name(), 20*time.Millisecond)
	
	result3, err := executor3.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, result3.Success)
	
	// Give some time for final visualization update
	time.Sleep(100 * time.Millisecond)
	
	// Check that text file was created and contains data
	txtData, err := os.ReadFile(txtFile.Name())
	assert.NoError(t, err)
	assert.NotEmpty(t, txtData)
	assert.Contains(t, string(txtData), "Migration DAG Execution Summary")
	assert.Contains(t, string(txtData), "text-task")
}

func TestExecutor_VisualizationStatusChanges(t *testing.T) {
	dag := NewDAG()
	
	// Create a task that takes some time to complete
	task := &MockTask{
		id:          "slow-task",
		taskType:    "SlowTask",
		description: "Task that takes time",
		executeFunc: func(ctx context.Context) error {
			// Simulate work
			select {
			case <-time.After(200 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	
	node := NewBaseNode(task)
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Create executor
	config := &ExecutorConfig{
		MaxParallelTasks:  1,
		TaskTimeout:       5 * time.Second,
		PollInterval:      10 * time.Millisecond,
	}
	executor := NewExecutor(dag, config)
	
	// Create temporary file for visualization
	vizFile, err := os.CreateTemp("", "test_status_viz_*.json")
	require.NoError(t, err)
	defer os.Remove(vizFile.Name())
	vizFile.Close()
	
	// Enable visualization with frequent updates
	executor.EnableVisualization(vizFile.Name(), 50*time.Millisecond)
	
	// Execute in a goroutine so we can check intermediate states
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := executor.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result.Success)
	}()
	
	// Wait a bit for execution to start
	time.Sleep(100 * time.Millisecond)
	
	// Check that visualization file exists and is being updated
	_, err = os.Stat(vizFile.Name())
	assert.NoError(t, err)
	
	// Wait for execution to complete
	select {
	case <-done:
		// Execution completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("Execution took too long")
	}
	
	// Give time for final update
	time.Sleep(100 * time.Millisecond)
	
	// Check final visualization contains completed status
	finalData, err := os.ReadFile(vizFile.Name())
	assert.NoError(t, err)
	assert.Contains(t, string(finalData), "slow-task")
	assert.Contains(t, string(finalData), "\"completedNodes\": 1") // JSON should show completion
}

func TestExecutor_VisualizationWithFailure(t *testing.T) {
	dag := NewDAG()
	
	// Create a task that fails
	task := &MockTask{
		id:          "failing-task",
		taskType:    "FailingTask",
		description: "Task that will fail",
		executeFunc: func(ctx context.Context) error {
			return assert.AnError
		},
	}
	
	node := NewBaseNode(task)
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Create executor
	config := &ExecutorConfig{
		MaxParallelTasks:  1,
		TaskTimeout:       5 * time.Second,
		PollInterval:      10 * time.Millisecond,
	}
	executor := NewExecutor(dag, config)
	
	// Create temporary file for visualization
	vizFile, err := os.CreateTemp("", "test_failure_viz_*.json")
	require.NoError(t, err)
	defer os.Remove(vizFile.Name())
	vizFile.Close()
	
	// Enable visualization
	executor.EnableVisualization(vizFile.Name(), 50*time.Millisecond)
	
	ctx := context.Background()
	result, err := executor.Execute(ctx)
	require.NoError(t, err)
	assert.False(t, result.Success) // Should fail
	
	// Give time for final update
	time.Sleep(100 * time.Millisecond)
	
	// Check visualization shows failure
	finalData, err := os.ReadFile(vizFile.Name())
	assert.NoError(t, err)
	assert.Contains(t, string(finalData), "failing-task")
	assert.Contains(t, string(finalData), "\"failedNodes\": 1") // JSON should show failure
}

func TestExecutor_ManualVisualizationExport(t *testing.T) {
	dag := NewDAG()
	
	task := &MockTask{
		id:          "manual-task",
		taskType:    "ManualTask",
		description: "Task for manual export test",
	}
	
	node := NewBaseNode(task)
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Create executor
	executor := NewExecutor(dag, DefaultExecutorConfig())
	
	// Get visualization and manually export
	viz := executor.GetVisualization()
	
	// Test manual JSON export
	jsonFile, err := os.CreateTemp("", "manual_export_*.json")
	require.NoError(t, err)
	defer os.Remove(jsonFile.Name())
	jsonFile.Close()
	
	err = viz.ExportToJSON(jsonFile.Name())
	assert.NoError(t, err)
	
	jsonData, err := os.ReadFile(jsonFile.Name())
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "manual-task")
	
	// Test manual DOT export
	dotFile, err := os.CreateTemp("", "manual_export_*.dot")
	require.NoError(t, err)
	defer os.Remove(dotFile.Name())
	dotFile.Close()
	
	err = viz.ExportToDOT(dotFile.Name())
	assert.NoError(t, err)
	
	dotData, err := os.ReadFile(dotFile.Name())
	assert.NoError(t, err)
	assert.Contains(t, string(dotData), "manual-task")
	assert.Contains(t, string(dotData), "digraph MigrationDAG")
}

func TestExecutor_VisualizationNoFile(t *testing.T) {
	dag := NewDAG()
	
	task := &MockTask{
		id:          "no-file-task",
		taskType:    "NoFileTask",
		description: "Task without visualization file",
	}
	
	node := NewBaseNode(task)
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Create executor
	executor := NewExecutor(dag, DefaultExecutorConfig())
	
	// Don't enable visualization
	ctx := context.Background()
	result, err := executor.Execute(ctx)
	require.NoError(t, err)
	assert.True(t, result.Success)
	
	// This should not crash even though no visualization is enabled
	assert.Equal(t, "", executor.visualizeFile)
}