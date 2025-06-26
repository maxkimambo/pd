package taskmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTask is a test implementation of the Task interface
type mockTask struct {
	id   string
	name string
}

// Abort implements BaseTask.
func (m *mockTask) Abort() {
	panic("unimplemented")
}

// Finish implements BaseTask.
func (m *mockTask) Finish() {
	panic("unimplemented")
}

// Start implements BaseTask.
func (m *mockTask) Start() {
	panic("unimplemented")
}

func (m *mockTask) ID() string {
	return m.id
}

func (m *mockTask) Name() string {
	return m.name
}

func (m *mockTask) Execute() (TaskResult, error) {
	return TaskResult{
		TaskID:  m.id,
		Success: true,
		Error:   nil,
		Status:  "completed",
	}, nil
}

func (m *mockTask) Status() string {
	return "ready"
}

func newMockTask(id, name string) *mockTask {
	return &mockTask{
		id:   id,
		name: name,
	}
}

// Helper function to create a test execution function
func createTestExecute(id string, success bool) func() (TaskResult, error) {
	
	t := NewTask()
	return func() (TaskResult, error) {
		result := TaskResult{
			TaskID:  id,
			Success: success,
			Status:  "completed",
		}
		if !success {
			result.Status = "failed"
		}
		return result, nil
	}
}

func TestNewNode(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		nodeName string
		nodeType string
	}{
		{
			name:     "create valid node",
			id:       "node1",
			nodeName: "Test Node",
			nodeType: "compute",
		},
		{
			name:     "create node with empty strings",
			id:       "",
			nodeName: "",
			nodeType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execFunc := createTestExecute(tt.id, true)
			node := NewNode(tt.id, tt.nodeName, execFunc)

			assert.Equal(t, tt.id, node.ID)
			assert.Equal(t, tt.nodeName, node.Name)
			assert.Equal(t, tt.nodeType, node.Type)
			assert.NotNil(t, node.Execute)

			// Test execution
			result, err := node.Execute()
			assert.NoError(t, err)
			assert.Equal(t, tt.id, result.TaskID)
			assert.True(t, result.Success)
		})
	}
}

func TestNewDAG(t *testing.T) {
	dag := NewDAG()

	assert.NotNil(t, dag)
	assert.NotNil(t, dag.Nodes)
	assert.NotNil(t, dag.Edges)
	assert.Equal(t, 0, len(dag.Nodes))
	assert.Equal(t, 0, len(dag.Edges))
	assert.Equal(t, 0, len(dag.Roots))
	assert.False(t, dag.isFinalized)
}

func TestDAG_AddNode(t *testing.T) {
	tests := []struct {
		name        string
		setupDAG    func() *DAG
		node        *Node
		expectError bool
		errorMsg    string
	}{
		{
			name: "add valid node",
			setupDAG: func() *DAG {
				return NewDAG()
			},
			node:        NewNode("node1", "Test Node", "compute", createTestExecute("node1", true)),
			expectError: false,
		},
		{
			name: "add nil node",
			setupDAG: func() *DAG {
				return NewDAG()
			},
			node:        nil,
			expectError: true,
			errorMsg:    "node is nil",
		},
		{
			name: "add duplicate node",
			setupDAG: func() *DAG {
				dag := NewDAG()
				node := NewNode("node1", "Test Node", "compute", createTestExecute("node1", true))
				dag.AddNode(node)
				return dag
			},
			node:        NewNode("node1", "Duplicate Node", "compute", createTestExecute("node1", true)),
			expectError: true,
			errorMsg:    "node with ID node1 already exists",
		},
		{
			name: "add node to finalized DAG",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.Finalize()
				return dag
			},
			node:        NewNode("node1", "Test Node", "compute", createTestExecute("node1", true)),
			expectError: true,
			errorMsg:    "DAG is finalized, no further modifications allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			err := dag.AddNode(tt.node)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, dag.Nodes, tt.node.ID)
				assert.Equal(t, tt.node, dag.Nodes[tt.node.ID])
			}
		})
	}
}

func TestDAG_AddDependency(t *testing.T) {
	tests := []struct {
		name        string
		setupDAG    func() *DAG
		fromID      string
		toID        string
		expectError bool
		errorMsg    string
	}{
		{
			name: "add valid dependency",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				return dag
			},
			fromID:      "node1",
			toID:        "node2",
			expectError: false,
		},
		{
			name: "add dependency with non-existent source node",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				return dag
			},
			fromID:      "node1",
			toID:        "node2",
			expectError: true,
			errorMsg:    "source node node1 does not exist",
		},
		{
			name: "add dependency with non-existent target node",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				return dag
			},
			fromID:      "node1",
			toID:        "node2",
			expectError: true,
			errorMsg:    "target node node2 does not exist",
		},
		{
			name: "add self dependency",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				return dag
			},
			fromID:      "node1",
			toID:        "node1",
			expectError: true,
			errorMsg:    "cannot add dependency from a node to itself: node1",
		},
		{
			name: "add duplicate dependency",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddDependency("node1", "node2")
				return dag
			},
			fromID:      "node1",
			toID:        "node2",
			expectError: true,
			errorMsg:    "duplicate dependency found from node1 to node2",
		},
		{
			name: "add dependency to finalized DAG",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.Finalize()
				return dag
			},
			fromID:      "node1",
			toID:        "node2",
			expectError: true,
			errorMsg:    "DAG is finalized, no further modifications allowed",
		},
		{
			name: "add dependency that creates cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node2", "node3")
				return dag
			},
			fromID:      "node3",
			toID:        "node1",
			expectError: true,
			errorMsg:    "adding dependency from node3 to node1 would create a cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			err := dag.AddDependency(tt.fromID, tt.toID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				deps, _ := dag.GetDependencies(tt.fromID)
				assert.Contains(t, deps, tt.toID)
			}
		})
	}
}

func TestDAG_GetDependencies(t *testing.T) {
	tests := []struct {
		name        string
		setupDAG    func() *DAG
		nodeID      string
		expected    []string
		expectError bool
		errorMsg    string
	}{
		{
			name: "get dependencies for node with dependencies",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node1", "node3")
				return dag
			},
			nodeID:      "node1",
			expected:    []string{"node2", "node3"},
			expectError: false,
		},
		{
			name: "get dependencies for node with no dependencies",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				return dag
			},
			nodeID:      "node1",
			expected:    []string{},
			expectError: false,
		},
		{
			name: "get dependencies for non-existent node",
			setupDAG: func() *DAG {
				return NewDAG()
			},
			nodeID:      "node1",
			expectError: true,
			errorMsg:    "node node1 does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			deps, err := dag.GetDependencies(tt.nodeID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, deps)
			} else {
				assert.NoError(t, err)
				if tt.expected == nil {
					tt.expected = []string{}
				}
				if deps == nil {
					deps = []string{}
				}
				assert.ElementsMatch(t, tt.expected, deps)
			}
		})
	}
}

func TestDAG_GetNode(t *testing.T) {
	tests := []struct {
		name        string
		setupDAG    func() *DAG
		nodeID      string
		expectError bool
		errorMsg    string
	}{
		{
			name: "get existing node",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				return dag
			},
			nodeID:      "node1",
			expectError: false,
		},
		{
			name: "get non-existent node",
			setupDAG: func() *DAG {
				return NewDAG()
			},
			nodeID:      "node1",
			expectError: true,
			errorMsg:    "node node1 does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			node, err := dag.GetNode(tt.nodeID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.Equal(t, tt.nodeID, node.ID)
			}
		})
	}
}

func TestDAG_Finalize(t *testing.T) {
	tests := []struct {
		name     string
		setupDAG func() *DAG
		validate func(t *testing.T, dag *DAG)
	}{
		{
			name: "finalize empty DAG",
			setupDAG: func() *DAG {
				return NewDAG()
			},
			validate: func(t *testing.T, dag *DAG) {
				assert.True(t, dag.isFinalized)
				assert.Equal(t, 0, len(dag.Roots))
				assert.NotNil(t, dag.indegrees)
			},
		},
		{
			name: "finalize DAG with single node",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				return dag
			},
			validate: func(t *testing.T, dag *DAG) {
				assert.True(t, dag.isFinalized)
				assert.Equal(t, 1, len(dag.Roots))
				assert.Equal(t, "node1", dag.Roots[0].ID)
				assert.Equal(t, 0, dag.indegrees["node1"])
			},
		},
		{
			name: "finalize DAG with dependencies",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node2", "node3")
				return dag
			},
			validate: func(t *testing.T, dag *DAG) {
				assert.True(t, dag.isFinalized)
				assert.Equal(t, 1, len(dag.Roots))
				assert.Equal(t, "node1", dag.Roots[0].ID)
				assert.Equal(t, 0, dag.indegrees["node1"])
				assert.Equal(t, 1, dag.indegrees["node2"])
				assert.Equal(t, 1, dag.indegrees["node3"])
			},
		},
		{
			name: "finalize DAG with multiple roots",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node3")
				dag.AddDependency("node2", "node3")
				return dag
			},
			validate: func(t *testing.T, dag *DAG) {
				assert.True(t, dag.isFinalized)
				assert.Equal(t, 2, len(dag.Roots))

				rootIDs := make([]string, len(dag.Roots))
				for i, root := range dag.Roots {
					rootIDs[i] = root.ID
				}
				assert.ElementsMatch(t, []string{"node1", "node2"}, rootIDs)

				assert.Equal(t, 0, dag.indegrees["node1"])
				assert.Equal(t, 0, dag.indegrees["node2"])
				assert.Equal(t, 2, dag.indegrees["node3"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			dag.Finalize()
			tt.validate(t, dag)
		})
	}
}

func TestDAG_hasCycle(t *testing.T) {
	tests := []struct {
		name        string
		setupDAG    func() *DAG
		expectCycle bool
	}{
		{
			name: "empty DAG has no cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.Finalize()
				return dag
			},
			expectCycle: false,
		},
		{
			name: "single node DAG has no cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.Finalize()
				return dag
			},
			expectCycle: false,
		},
		{
			name: "linear DAG has no cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node2", "node3")
				dag.Finalize()
				return dag
			},
			expectCycle: false,
		},
		{
			name: "DAG with diamond pattern has no cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddNode(NewNode("node4", "Node 4", "compute", createTestExecute("node4", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node1", "node3")
				dag.AddDependency("node2", "node4")
				dag.AddDependency("node3", "node4")
				dag.Finalize()
				return dag
			},
			expectCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			hasCycle := dag.hasCycle()
			assert.Equal(t, tt.expectCycle, hasCycle)
		})
	}
}

func TestDAG_createsACycle(t *testing.T) {
	tests := []struct {
		name         string
		setupDAG     func() *DAG
		fromNode     string
		toNode       string
		expectsCycle bool
	}{
		{
			name: "adding edge that creates no cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				return dag
			},
			fromNode:     "node1",
			toNode:       "node2",
			expectsCycle: false,
		},
		{
			name: "adding edge that creates simple cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddDependency("node1", "node2")
				return dag
			},
			fromNode:     "node2",
			toNode:       "node1",
			expectsCycle: true,
		},
		{
			name: "adding edge that creates complex cycle",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node2", "node3")
				return dag
			},
			fromNode:     "node3",
			toNode:       "node1",
			expectsCycle: true,
		},
		{
			name: "adding edge between independent nodes",
			setupDAG: func() *DAG {
				dag := NewDAG()
				dag.AddNode(NewNode("node1", "Node 1", "compute", createTestExecute("node1", true)))
				dag.AddNode(NewNode("node2", "Node 2", "compute", createTestExecute("node2", true)))
				dag.AddNode(NewNode("node3", "Node 3", "compute", createTestExecute("node3", true)))
				dag.AddNode(NewNode("node4", "Node 4", "compute", createTestExecute("node4", true)))
				dag.AddDependency("node1", "node2")
				dag.AddDependency("node3", "node4")
				return dag
			},
			fromNode:     "node2",
			toNode:       "node3",
			expectsCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := tt.setupDAG()
			createsCycle := dag.createsACycle(tt.fromNode, tt.toNode)
			assert.Equal(t, tt.expectsCycle, createsCycle)
		})
	}
}

func TestDAG_Integration(t *testing.T) {
	t.Run("complex DAG operations", func(t *testing.T) {
		dag := NewDAG()

		// Add nodes
		nodes := []string{"A", "B", "C", "D", "E", "F"}
		for _, nodeID := range nodes {
			node := NewNode(nodeID, "Node "+nodeID, "compute", createTestExecute(nodeID, true))
			err := dag.AddNode(node)
			require.NoError(t, err)
		}

		// Add dependencies to create a complex DAG:
		// A -> B -> D
		// A -> C -> D
		// B -> E
		// C -> F
		// E -> F
		dependencies := map[string][]string{
			"A": {"B", "C"},
			"B": {"D", "E"},
			"C": {"D", "F"},
			"E": {"F"},
		}

		for from, tos := range dependencies {
			for _, to := range tos {
				err := dag.AddDependency(from, to)
				require.NoError(t, err)
			}
		}

		// Verify dependencies
		for from, expectedTos := range dependencies {
			actualTos, err := dag.GetDependencies(from)
			require.NoError(t, err)
			assert.ElementsMatch(t, expectedTos, actualTos)
		}

		// Verify all nodes exist
		for _, nodeID := range nodes {
			node, err := dag.GetNode(nodeID)
			require.NoError(t, err)
			assert.Equal(t, nodeID, node.ID)
		}

		// Finalize and verify structure
		dag.Finalize()
		assert.True(t, dag.isFinalized)
		assert.Equal(t, 1, len(dag.Roots))
		assert.Equal(t, "A", dag.Roots[0].ID)

		// Verify indegrees
		expectedIndegrees := map[string]int{
			"A": 0, "B": 1, "C": 1, "D": 2, "E": 1, "F": 2,
		}
		for nodeID, expectedIndegree := range expectedIndegrees {
			assert.Equal(t, expectedIndegree, dag.indegrees[nodeID], "Node %s should have indegree %d", nodeID, expectedIndegree)
		}

		// Verify no cycles
		assert.False(t, dag.hasCycle())

		// Try to modify finalized DAG (should fail)
		newNode := NewNode("G", "Node G", "compute", createTestExecute("G", true))
		err := dag.AddNode(newNode)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DAG is finalized")

		err = dag.AddDependency("A", "F")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DAG is finalized")
	})
}
