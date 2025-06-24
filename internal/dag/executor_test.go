package dag

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controllableTask allows precise control over execution behavior for testing
type controllableTask struct {
	*BaseTask
	executeFunc func(ctx context.Context) (*TaskResult, error)
	delay       time.Duration
	shouldFail  bool
	executed    bool
	mutex       sync.Mutex
}

func newControllableTask(id, taskType, description string, delay time.Duration) *controllableTask {
	return &controllableTask{
		BaseTask: NewBaseTask(id, description, taskType),
		delay:    delay,
	}
}

func (c *controllableTask) Execute(ctx context.Context) (*TaskResult, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	result := NewTaskResult(c.GetID(), c.GetName())
	result.MarkStarted()

	if c.executed {
		err := errors.New("task already executed")
		result.MarkFailed(err)
		return result, err
	}

	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			err := ctx.Err()
			result.MarkFailed(err)
			return result, err
		}
	}

	if c.executeFunc != nil {
		return c.executeFunc(ctx)
	}

	if c.shouldFail {
		err := errors.New("task configured to fail")
		result.MarkFailed(err)
		return result, err
	}

	c.executed = true
	result.MarkCompleted()
	return result, nil
}

func (c *controllableTask) WasExecuted() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.executed
}

func TestDefaultExecutorConfig(t *testing.T) {
	config := DefaultExecutorConfig()

	assert.Equal(t, 10, config.MaxParallelTasks)
	assert.Equal(t, 15*time.Minute, config.TaskTimeout)
	assert.Equal(t, 100*time.Millisecond, config.PollInterval)
}

func TestNodeResult(t *testing.T) {
	start := time.Now()
	end := start.Add(100 * time.Millisecond)

	result := &NodeResult{
		NodeID:    "test-node",
		Success:   true,
		Error:     nil,
		StartTime: &start,
		EndTime:   &end,
		Duration:  100 * time.Millisecond,
	}

	assert.Equal(t, "test-node", result.NodeID)
	assert.True(t, result.Success)
	assert.NoError(t, result.Error)
	assert.Equal(t, start, *result.StartTime)
	assert.Equal(t, end, *result.EndTime)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
}

func TestExecutionResult(t *testing.T) {
	result := &ExecutionResult{
		Success:       true,
		NodeResults:   make(map[string]*NodeResult),
		ExecutionTime: 500 * time.Millisecond,
		Error:         nil,
	}

	assert.True(t, result.Success)
	assert.NotNil(t, result.NodeResults)
	assert.Equal(t, 500*time.Millisecond, result.ExecutionTime)
	assert.NoError(t, result.Error)
}

func TestNewExecutor(t *testing.T) {
	dag := NewDAG()

	t.Run("with default config", func(t *testing.T) {
		executor := NewExecutor(dag, nil)
		assert.NotNil(t, executor)
		assert.Equal(t, dag, executor.dag)
		assert.Equal(t, DefaultExecutorConfig().MaxParallelTasks, cap(executor.workers))
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &ExecutorConfig{
			MaxParallelTasks: 5,
			TaskTimeout:      5 * time.Minute,
			PollInterval:     50 * time.Millisecond,
		}

		executor := NewExecutor(dag, config)
		assert.NotNil(t, executor)
		assert.Equal(t, dag, executor.dag)
		assert.Equal(t, config, executor.config)
		assert.Equal(t, 5, cap(executor.workers))
	})
}

func TestExecutor_Execute_SingleNode(t *testing.T) {
	dag := NewDAG()
	task := newControllableTask("task-1", "test", "Single task", 10*time.Millisecond)
	node := NewBaseNode(task)

	err := dag.AddNode(node)
	require.NoError(t, err)

	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Len(t, result.NodeResults, 1)
	assert.True(t, task.WasExecuted())

	nodeResult := result.NodeResults["task-1"]
	assert.NotNil(t, nodeResult)
	assert.True(t, nodeResult.Success)
	assert.NoError(t, nodeResult.Error)
	assert.NotNil(t, nodeResult.StartTime)
	assert.NotNil(t, nodeResult.EndTime)
	assert.True(t, nodeResult.Duration >= 10*time.Millisecond)
}

func TestExecutor_Execute_ParallelNodes(t *testing.T) {
	dag := NewDAG()

	// Create three independent tasks
	task1 := newControllableTask("task-1", "test", "Task 1", 20*time.Millisecond)
	task2 := newControllableTask("task-2", "test", "Task 2", 20*time.Millisecond)
	task3 := newControllableTask("task-3", "test", "Task 3", 20*time.Millisecond)

	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)

	require.NoError(t, dag.AddNode(node1))
	require.NoError(t, dag.AddNode(node2))
	require.NoError(t, dag.AddNode(node3))

	executor := NewExecutor(dag, DefaultExecutorConfig())
	start := time.Now()
	result, err := executor.Execute(context.Background())
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.NodeResults, 3)

	// Should execute in parallel, so total time should be close to individual task time
	assert.True(t, duration < 60*time.Millisecond, "Expected parallel execution to be faster")

	// All tasks should have been executed
	assert.True(t, task1.WasExecuted())
	assert.True(t, task2.WasExecuted())
	assert.True(t, task3.WasExecuted())
}

func TestExecutor_Execute_WithDependencies(t *testing.T) {
	dag := NewDAG()

	// Create tasks: task1 -> task2 -> task3
	task1 := newControllableTask("task-1", "test", "Task 1", 10*time.Millisecond)
	task2 := newControllableTask("task-2", "test", "Task 2", 10*time.Millisecond)
	task3 := newControllableTask("task-3", "test", "Task 3", 10*time.Millisecond)

	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)

	require.NoError(t, dag.AddNode(node1))
	require.NoError(t, dag.AddNode(node2))
	require.NoError(t, dag.AddNode(node3))

	// Add dependencies: task1 -> task2 -> task3
	require.NoError(t, dag.AddDependency("task-1", "task-2"))
	require.NoError(t, dag.AddDependency("task-2", "task-3"))

	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.NodeResults, 3)

	// All tasks should have been executed
	assert.True(t, task1.WasExecuted())
	assert.True(t, task2.WasExecuted())
	assert.True(t, task3.WasExecuted())

	// Check execution order by comparing start times
	result1 := result.NodeResults["task-1"]
	result2 := result.NodeResults["task-2"]
	result3 := result.NodeResults["task-3"]

	assert.True(t, result1.StartTime.Before(*result2.StartTime))
	assert.True(t, result2.StartTime.Before(*result3.StartTime))
}

func TestExecutor_Execute_TaskFailure(t *testing.T) {
	dag := NewDAG()

	// Create tasks where task2 will fail
	task1 := newControllableTask("task-1", "test", "Task 1", 10*time.Millisecond)
	task2 := newControllableTask("task-2", "test", "Task 2", 10*time.Millisecond)
	task3 := newControllableTask("task-3", "test", "Task 3", 10*time.Millisecond)

	task2.shouldFail = true

	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)

	require.NoError(t, dag.AddNode(node1))
	require.NoError(t, dag.AddNode(node2))
	require.NoError(t, dag.AddNode(node3))

	// Dependencies: task1 -> task2 -> task3
	require.NoError(t, dag.AddDependency("task-1", "task-2"))
	require.NoError(t, dag.AddDependency("task-2", "task-3"))

	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.NoError(t, err) // Execute doesn't return error, but result contains failures
	assert.False(t, result.Success)
	assert.NotNil(t, result.Error)

	// Task1 should succeed, task2 should fail, task3 should be cancelled
	assert.True(t, task1.WasExecuted())
	assert.False(t, task2.WasExecuted()) // Fails during execution
	assert.False(t, task3.WasExecuted()) // Cancelled due to dependency failure

	assert.True(t, result.NodeResults["task-1"].Success)
	assert.False(t, result.NodeResults["task-2"].Success)
	assert.False(t, result.NodeResults["task-3"].Success)
	assert.Contains(t, result.NodeResults["task-3"].Error.Error(), "cancelled")
}

func TestExecutor_Execute_ContextCancellation(t *testing.T) {
	dag := NewDAG()

	// Create a long-running task
	task := newControllableTask("task-1", "test", "Long task", 1*time.Second)
	node := NewBaseNode(task)

	require.NoError(t, dag.AddNode(node))

	executor := NewExecutor(dag, DefaultExecutorConfig())

	// Create context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := executor.Execute(ctx)

	assert.NoError(t, err) // Execute doesn't return error for context cancellation
	assert.False(t, result.Success)

	// Task should not have completed
	assert.False(t, task.WasExecuted())

	nodeResult := result.NodeResults["task-1"]
	assert.False(t, nodeResult.Success)
	assert.ErrorIs(t, nodeResult.Error, context.Canceled)
}

func TestExecutor_Execute_TaskTimeout(t *testing.T) {
	dag := NewDAG()

	// Create a task that takes longer than the timeout
	task := newControllableTask("task-1", "test", "Slow task", 200*time.Millisecond)
	node := NewBaseNode(task)

	require.NoError(t, dag.AddNode(node))

	// Set a very short task timeout
	config := DefaultExecutorConfig()
	config.TaskTimeout = 50 * time.Millisecond

	executor := NewExecutor(dag, config)
	result, err := executor.Execute(context.Background())

	assert.NoError(t, err)
	assert.False(t, result.Success)

	nodeResult := result.NodeResults["task-1"]
	assert.False(t, nodeResult.Success)
	assert.ErrorIs(t, nodeResult.Error, context.DeadlineExceeded)
}

func TestExecutor_Execute_InvalidDAG(t *testing.T) {
	dag := NewDAG()

	// Create circular dependency
	task1 := newControllableTask("task-1", "test", "Task 1", 10*time.Millisecond)
	task2 := newControllableTask("task-2", "test", "Task 2", 10*time.Millisecond)

	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)

	require.NoError(t, dag.AddNode(node1))
	require.NoError(t, dag.AddNode(node2))
	require.NoError(t, dag.AddDependency("task-1", "task-2"))
	require.NoError(t, dag.AddDependency("task-2", "task-1"))

	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, err.Error(), "invalid DAG")
}

func TestExecutor_Execute_EmptyDAG(t *testing.T) {
	dag := NewDAG()

	// Create empty DAG (no nodes at all)
	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no root nodes")
	assert.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestExecutor_GetProgress(t *testing.T) {
	dag := NewDAG()

	task1 := newControllableTask("task-1", "test", "Task 1", 100*time.Millisecond)
	task2 := newControllableTask("task-2", "test", "Task 2", 100*time.Millisecond)

	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)

	require.NoError(t, dag.AddNode(node1))
	require.NoError(t, dag.AddNode(node2))

	executor := NewExecutor(dag, DefaultExecutorConfig())

	// Initially no progress
	completed, total := executor.GetProgress()
	assert.Equal(t, 0, completed)
	assert.Equal(t, 0, total)

	// Start execution in background
	go func() {
		_, _ = executor.Execute(context.Background())
	}()

	// Give it time to initialize
	time.Sleep(10 * time.Millisecond)

	// Should show total tasks but maybe not completed yet
	completed, total = executor.GetProgress()
	assert.Equal(t, 2, total)
	assert.True(t, completed >= 0 && completed <= 2)
}

func TestExecutor_Cancel(t *testing.T) {
	dag := NewDAG()

	// Create a long-running task
	task := newControllableTask("task-1", "test", "Long task", 500*time.Millisecond)
	node := NewBaseNode(task)

	require.NoError(t, dag.AddNode(node))

	executor := NewExecutor(dag, DefaultExecutorConfig())

	// Start execution in background
	resultChan := make(chan *ExecutionResult)
	go func() {
		result, _ := executor.Execute(context.Background())
		resultChan <- result
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel execution
	executor.Cancel()

	// Wait for result
	result := <-resultChan

	assert.False(t, result.Success)
	assert.False(t, task.WasExecuted())
}

func TestExecutor_IsRunning(t *testing.T) {
	dag := NewDAG()

	task := newControllableTask("task-1", "test", "Task", 100*time.Millisecond)
	node := NewBaseNode(task)

	require.NoError(t, dag.AddNode(node))

	executor := NewExecutor(dag, DefaultExecutorConfig())

	// Initially not running
	assert.False(t, executor.IsRunning())

	// Start execution
	done := make(chan bool)
	go func() {
		_, _ = executor.Execute(context.Background())
		done <- true
	}()

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Should be running
	assert.True(t, executor.IsRunning())

	// Wait for completion
	<-done

	// Should no longer be running
	assert.False(t, executor.IsRunning())
}

func TestExecutor_Execute_ComplexDAG(t *testing.T) {
	dag := NewDAG()

	// Create a diamond-shaped DAG:
	//     A
	//   /   \
	//  B     C
	//   \   /
	//     D

	taskA := newControllableTask("A", "test", "Task A", 10*time.Millisecond)
	taskB := newControllableTask("B", "test", "Task B", 10*time.Millisecond)
	taskC := newControllableTask("C", "test", "Task C", 10*time.Millisecond)
	taskD := newControllableTask("D", "test", "Task D", 10*time.Millisecond)

	nodeA := NewBaseNode(taskA)
	nodeB := NewBaseNode(taskB)
	nodeC := NewBaseNode(taskC)
	nodeD := NewBaseNode(taskD)

	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.AddNode(nodeB))
	require.NoError(t, dag.AddNode(nodeC))
	require.NoError(t, dag.AddNode(nodeD))

	// Dependencies: A -> B, A -> C, B -> D, C -> D
	require.NoError(t, dag.AddDependency("A", "B"))
	require.NoError(t, dag.AddDependency("A", "C"))
	require.NoError(t, dag.AddDependency("B", "D"))
	require.NoError(t, dag.AddDependency("C", "D"))

	executor := NewExecutor(dag, DefaultExecutorConfig())
	result, err := executor.Execute(context.Background())

	assert.NoError(t, err)
	if !result.Success {
		t.Logf("Execution failed. Error: %v", result.Error)
		for nodeID, nodeResult := range result.NodeResults {
			t.Logf("Node %s: Success=%v, Error=%v", nodeID, nodeResult.Success, nodeResult.Error)
		}
	}
	assert.True(t, result.Success)
	assert.Len(t, result.NodeResults, 4)

	// All tasks should have been executed
	assert.True(t, taskA.WasExecuted())
	assert.True(t, taskB.WasExecuted())
	assert.True(t, taskC.WasExecuted())
	assert.True(t, taskD.WasExecuted())

	// Check execution order
	resultA := result.NodeResults["A"]
	resultB := result.NodeResults["B"]
	resultC := result.NodeResults["C"]
	resultD := result.NodeResults["D"]

	// A must execute before B and C
	assert.True(t, resultA.StartTime.Before(*resultB.StartTime))
	assert.True(t, resultA.StartTime.Before(*resultC.StartTime))

	// D must execute after both B and C
	assert.True(t, resultB.EndTime.Before(*resultD.StartTime) || resultB.EndTime.Equal(*resultD.StartTime))
	assert.True(t, resultC.EndTime.Before(*resultD.StartTime) || resultC.EndTime.Equal(*resultD.StartTime))
}
