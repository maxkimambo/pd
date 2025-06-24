package dag

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTask is a test implementation of the Task interface
type mockTask struct {
	*BaseTask
	executeFunc func(ctx context.Context) error
	rollbackFunc func(ctx context.Context) error
}

func newMockTask(id, taskType, description string) *mockTask {
	return &mockTask{
		BaseTask: NewBaseTask(id, taskType, description),
	}
}

func (m *mockTask) Execute(ctx context.Context) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}
	return nil
}

func (m *mockTask) Rollback(ctx context.Context) error {
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx)
	}
	return nil
}

// mockNode is a test implementation of the Node interface
type mockNode struct {
	*BaseNode
}

func newMockNode(id string) *mockNode {
	task := newMockTask(id, "test", "Test task")
	return &mockNode{
		BaseNode: NewBaseNode(task),
	}
}

func newMockNodeWithTask(task Task) *mockNode {
	return &mockNode{
		BaseNode: NewBaseNode(task),
	}
}

func TestNewDAG(t *testing.T) {
	dag := NewDAG()
	assert.NotNil(t, dag)
	assert.Equal(t, 0, dag.Size())
}

func TestDAG_AddNode(t *testing.T) {
	tests := []struct {
		name      string
		node      Node
		expectErr bool
	}{
		{
			name:      "valid node",
			node:      newMockNode("test-1"),
			expectErr: false,
		},
		{
			name:      "nil node",
			node:      nil,
			expectErr: true,
		},
		{
			name:      "empty ID",
			node:      newMockNode(""),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := NewDAG()
			err := dag.AddNode(tt.node)
			
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, 1, dag.Size())
			}
		})
	}
}

func TestDAG_AddDuplicateNode(t *testing.T) {
	dag := NewDAG()
	node := newMockNode("test-1")
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Adding the same node should fail
	err = dag.AddNode(node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestDAG_GetNode(t *testing.T) {
	dag := NewDAG()
	node := newMockNode("test-1")
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Get existing node
	retrievedNode, err := dag.GetNode("test-1")
	assert.NoError(t, err)
	assert.Equal(t, node, retrievedNode)
	
	// Get non-existent node
	_, err = dag.GetNode("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDAG_AddDependency(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	
	// Add valid dependency
	err = dag.AddDependency("node-1", "node-2")
	assert.NoError(t, err)
	
	// Add dependency with non-existent source
	err = dag.AddDependency("non-existent", "node-2")
	assert.Error(t, err)
	
	// Add dependency with non-existent target
	err = dag.AddDependency("node-1", "non-existent")
	assert.Error(t, err)
}

func TestDAG_GetDependencies(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	node3 := newMockNode("node-3")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	// Add dependencies: node-1 -> node-3, node-2 -> node-3
	err = dag.AddDependency("node-1", "node-3")
	require.NoError(t, err)
	err = dag.AddDependency("node-2", "node-3")
	require.NoError(t, err)
	
	deps, err := dag.GetDependencies("node-3")
	assert.NoError(t, err)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "node-1")
	assert.Contains(t, deps, "node-2")
	
	// Node with no dependencies
	deps, err = dag.GetDependencies("node-1")
	assert.NoError(t, err)
	assert.Empty(t, deps)
}

func TestDAG_GetDependents(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	node3 := newMockNode("node-3")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	// Add dependencies: node-1 -> node-2, node-1 -> node-3
	err = dag.AddDependency("node-1", "node-2")
	require.NoError(t, err)
	err = dag.AddDependency("node-1", "node-3")
	require.NoError(t, err)
	
	dependents, err := dag.GetDependents("node-1")
	assert.NoError(t, err)
	assert.Len(t, dependents, 2)
	assert.Contains(t, dependents, "node-2")
	assert.Contains(t, dependents, "node-3")
}

func TestDAG_GetRootNodes(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	node3 := newMockNode("node-3")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	// node-1 and node-2 -> node-3
	err = dag.AddDependency("node-1", "node-3")
	require.NoError(t, err)
	err = dag.AddDependency("node-2", "node-3")
	require.NoError(t, err)
	
	roots, err := dag.GetRootNodes()
	assert.NoError(t, err)
	assert.Len(t, roots, 2)
	assert.Contains(t, roots, "node-1")
	assert.Contains(t, roots, "node-2")
}

func TestDAG_GetReadyNodes(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	node3 := newMockNode("node-3")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	// node-1 -> node-3, node-2 -> node-3
	err = dag.AddDependency("node-1", "node-3")
	require.NoError(t, err)
	err = dag.AddDependency("node-2", "node-3")
	require.NoError(t, err)
	
	// Initially, only root nodes should be ready
	ready, err := dag.GetReadyNodes()
	assert.NoError(t, err)
	assert.Len(t, ready, 2)
	assert.Contains(t, ready, "node-1")
	assert.Contains(t, ready, "node-2")
	
	// Complete node-1
	node1.SetStatus(StatusCompleted)
	ready, err = dag.GetReadyNodes()
	assert.NoError(t, err)
	assert.Len(t, ready, 1)
	assert.Contains(t, ready, "node-2")
	
	// Complete node-2, now node-3 should be ready
	node2.SetStatus(StatusCompleted)
	ready, err = dag.GetReadyNodes()
	assert.NoError(t, err)
	assert.Len(t, ready, 1)
	assert.Contains(t, ready, "node-3")
}

func TestDAG_Validate(t *testing.T) {
	t.Run("valid DAG", func(t *testing.T) {
		dag := NewDAG()
		node1 := newMockNode("node-1")
		node2 := newMockNode("node-2")
		
		err := dag.AddNode(node1)
		require.NoError(t, err)
		err = dag.AddNode(node2)
		require.NoError(t, err)
		err = dag.AddDependency("node-1", "node-2")
		require.NoError(t, err)
		
		err = dag.Validate()
		assert.NoError(t, err)
	})
	
	t.Run("cyclic DAG", func(t *testing.T) {
		dag := NewDAG()
		node1 := newMockNode("node-1")
		node2 := newMockNode("node-2")
		
		err := dag.AddNode(node1)
		require.NoError(t, err)
		err = dag.AddNode(node2)
		require.NoError(t, err)
		
		// Create cycle: node-1 -> node-2 -> node-1
		err = dag.AddDependency("node-1", "node-2")
		require.NoError(t, err)
		err = dag.AddDependency("node-2", "node-1")
		require.NoError(t, err)
		
		err = dag.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cycles")
	})
}

func TestDAG_IsComplete(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	
	// Initially not complete
	assert.False(t, dag.IsComplete())
	
	// Complete one node
	node1.SetStatus(StatusCompleted)
	assert.False(t, dag.IsComplete())
	
	// Complete all nodes
	node2.SetStatus(StatusCompleted)
	assert.True(t, dag.IsComplete())
}

func TestDAG_HasFailed(t *testing.T) {
	dag := NewDAG()
	node1 := newMockNode("node-1")
	node2 := newMockNode("node-2")
	
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	
	// Initially no failures
	assert.False(t, dag.HasFailed())
	
	// Complete one node
	node1.SetStatus(StatusCompleted)
	assert.False(t, dag.HasFailed())
	
	// Fail one node
	node2.SetStatus(StatusFailed)
	assert.True(t, dag.HasFailed())
}

func TestDAG_ConcurrentAccess(t *testing.T) {
	dag := NewDAG()
	
	// Test concurrent node additions
	const numNodes = 100
	nodes := make([]*mockNode, numNodes)
	
	// Create nodes concurrently
	errChan := make(chan error, numNodes)
	for i := 0; i < numNodes; i++ {
		go func(id int) {
			node := newMockNode(fmt.Sprintf("node-%d", id))
			nodes[id] = node
			errChan <- dag.AddNode(node)
		}(i)
	}
	
	// Check all additions succeeded
	for i := 0; i < numNodes; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}
	
	assert.Equal(t, numNodes, dag.Size())
}

func TestNodeStatus_String(t *testing.T) {
	tests := []struct {
		status   NodeStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
		{NodeStatus(999), "unknown"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}