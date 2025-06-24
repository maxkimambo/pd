package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDAGVisualization(t *testing.T) {
	dag := NewDAG()
	viz := NewDAGVisualization(dag)
	
	assert.NotNil(t, viz)
	assert.Equal(t, dag, viz.dag)
}

func TestDAGVisualization_GenerateDAGInfo(t *testing.T) {
	dag := NewDAG()
	
	// Create a simple DAG with dependencies
	task1 := &MockTask{id: "task1", taskType: "TestTask", description: "First task"}
	task2 := &MockTask{id: "task2", taskType: "TestTask", description: "Second task"}
	task3 := &MockTask{id: "task3", taskType: "TestTask", description: "Third task"}
	
	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)
	
	// Add nodes to DAG
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	// Add dependencies: task1 -> task2 -> task3
	err = dag.AddDependency("task1", "task2")
	require.NoError(t, err)
	err = dag.AddDependency("task2", "task3")
	require.NoError(t, err)
	
	// Set some node statuses and times
	node1.SetStatus(StatusCompleted)
	startTime1 := time.Now().Add(-10 * time.Minute)
	endTime1 := time.Now().Add(-8 * time.Minute)
	node1.SetStartTime(&startTime1)
	node1.SetEndTime(&endTime1)
	
	node2.SetStatus(StatusRunning)
	startTime2 := time.Now().Add(-5 * time.Minute)
	node2.SetStartTime(&startTime2)
	
	node3.SetStatus(StatusPending)
	
	viz := NewDAGVisualization(dag)
	dagInfo, err := viz.GenerateDAGInfo()
	
	assert.NoError(t, err)
	assert.NotNil(t, dagInfo)
	
	// Check nodes
	assert.Len(t, dagInfo.Nodes, 3)
	
	// Find nodes by ID
	var nodeInfo1, nodeInfo2, nodeInfo3 *NodeInfo
	for i := range dagInfo.Nodes {
		switch dagInfo.Nodes[i].ID {
		case "task1":
			nodeInfo1 = &dagInfo.Nodes[i]
		case "task2":
			nodeInfo2 = &dagInfo.Nodes[i]
		case "task3":
			nodeInfo3 = &dagInfo.Nodes[i]
		}
	}
	
	require.NotNil(t, nodeInfo1)
	require.NotNil(t, nodeInfo2)
	require.NotNil(t, nodeInfo3)
	
	// Check node1 (completed)
	assert.Equal(t, "task1", nodeInfo1.ID)
	assert.Equal(t, "TestTask", nodeInfo1.Type)
	assert.Equal(t, "First task", nodeInfo1.Description)
	assert.Equal(t, StatusCompleted, nodeInfo1.Status)
	assert.NotNil(t, nodeInfo1.StartTime)
	assert.NotNil(t, nodeInfo1.EndTime)
	assert.NotEmpty(t, nodeInfo1.Duration)
	
	// Check node2 (running)
	assert.Equal(t, "task2", nodeInfo2.ID)
	assert.Equal(t, StatusRunning, nodeInfo2.Status)
	assert.NotNil(t, nodeInfo2.StartTime)
	assert.Nil(t, nodeInfo2.EndTime)
	assert.Contains(t, nodeInfo2.Duration, "running")
	
	// Check node3 (pending)
	assert.Equal(t, "task3", nodeInfo3.ID)
	assert.Equal(t, StatusPending, nodeInfo3.Status)
	assert.Nil(t, nodeInfo3.StartTime)
	assert.Nil(t, nodeInfo3.EndTime)
	assert.Empty(t, nodeInfo3.Duration)
	
	// Check edges
	assert.Len(t, dagInfo.Edges, 2)
	
	// Check stats
	assert.Equal(t, 3, dagInfo.Stats.TotalNodes)
	assert.Equal(t, 1, dagInfo.Stats.CompletedNodes)
	assert.Equal(t, 0, dagInfo.Stats.FailedNodes)
	assert.Equal(t, 1, dagInfo.Stats.RunningNodes)
	assert.Equal(t, 1, dagInfo.Stats.PendingNodes)
	assert.NotNil(t, dagInfo.Stats.StartTime)
	assert.NotEmpty(t, dagInfo.Stats.TotalDuration)
}

func TestDAGVisualization_ExportToJSON(t *testing.T) {
	dag := NewDAG()
	task := &MockTask{id: "test-task", taskType: "TestTask", description: "Test task"}
	node := NewBaseNode(task)
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	viz := NewDAGVisualization(dag)
	
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "dag_test_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	// Export to JSON
	err = viz.ExportToJSON(tmpFile.Name())
	assert.NoError(t, err)
	
	// Read and verify the JSON file
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	
	var dagInfo DAGInfo
	err = json.Unmarshal(data, &dagInfo)
	require.NoError(t, err)
	
	assert.Len(t, dagInfo.Nodes, 1)
	assert.Equal(t, "test-task", dagInfo.Nodes[0].ID)
	assert.Equal(t, "TestTask", dagInfo.Nodes[0].Type)
	assert.Equal(t, "Test task", dagInfo.Nodes[0].Description)
}

func TestDAGVisualization_GenerateDOTGraph(t *testing.T) {
	dag := NewDAG()
	
	// Create tasks with different statuses
	task1 := &MockTask{id: "completed", taskType: "CompletedTask", description: "Completed task"}
	task2 := &MockTask{id: "running", taskType: "RunningTask", description: "Running task"}
	task3 := &MockTask{id: "failed", taskType: "FailedTask", description: "Failed task"}
	task4 := &MockTask{id: "pending", taskType: "PendingTask", description: "Pending task"}
	
	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)
	node4 := NewBaseNode(task4)
	
	// Set different statuses
	node1.SetStatus(StatusCompleted)
	node2.SetStatus(StatusRunning)
	node3.SetStatus(StatusFailed)
	node4.SetStatus(StatusPending)
	
	// Add nodes to DAG
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	err = dag.AddNode(node4)
	require.NoError(t, err)
	
	// Add dependencies
	err = dag.AddDependency("completed", "running")
	require.NoError(t, err)
	err = dag.AddDependency("running", "failed")
	require.NoError(t, err)
	err = dag.AddDependency("running", "pending")
	require.NoError(t, err)
	
	viz := NewDAGVisualization(dag)
	dot, err := viz.GenerateDOTGraph()
	
	assert.NoError(t, err)
	assert.NotEmpty(t, dot)
	
	// Check that the DOT contains expected elements
	assert.Contains(t, dot, "digraph MigrationDAG")
	assert.Contains(t, dot, "\"completed\"")
	assert.Contains(t, dot, "\"running\"")
	assert.Contains(t, dot, "\"failed\"")
	assert.Contains(t, dot, "\"pending\"")
	
	// Check color coding
	assert.Contains(t, dot, "fillcolor=\"lightgreen\"") // completed
	assert.Contains(t, dot, "fillcolor=\"lightblue\"")  // running
	assert.Contains(t, dot, "fillcolor=\"salmon\"")     // failed
	assert.Contains(t, dot, "fillcolor=\"lightgrey\"")  // pending
	
	// Check dependencies
	assert.Contains(t, dot, "\"completed\" -> \"running\"")
	assert.Contains(t, dot, "\"running\" -> \"failed\"")
	assert.Contains(t, dot, "\"running\" -> \"pending\"")
	
	// Check legend
	assert.Contains(t, dot, "cluster_legend")
	assert.Contains(t, dot, "legend_pending")
	assert.Contains(t, dot, "legend_running")
	assert.Contains(t, dot, "legend_completed")
	assert.Contains(t, dot, "legend_failed")
	
	// Check stats
	assert.Contains(t, dot, "\"stats\"")
	assert.Contains(t, dot, "Total: 4")
}

func TestDAGVisualization_ExportToDOT(t *testing.T) {
	dag := NewDAG()
	task := &MockTask{id: "test-task", taskType: "TestTask", description: "Test task"}
	node := NewBaseNode(task)
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	viz := NewDAGVisualization(dag)
	
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "dag_test_*.dot")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	// Export to DOT
	err = viz.ExportToDOT(tmpFile.Name())
	assert.NoError(t, err)
	
	// Read and verify the DOT file
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	
	dotContent := string(data)
	assert.Contains(t, dotContent, "digraph MigrationDAG")
	assert.Contains(t, dotContent, "\"test-task\"")
	assert.Contains(t, dotContent, "TestTask")
}

func TestDAGVisualization_GenerateTextSummary(t *testing.T) {
	dag := NewDAG()
	
	// Create tasks with different statuses
	task1 := &MockTask{id: "completed", taskType: "CompletedTask", description: "Completed task"}
	task2 := &MockTask{id: "running", taskType: "RunningTask", description: "Running task"}
	task3 := &MockTask{id: "failed", taskType: "FailedTask", description: "Failed task"}
	
	node1 := NewBaseNode(task1)
	node2 := NewBaseNode(task2)
	node3 := NewBaseNode(task3)
	
	// Set different statuses
	node1.SetStatus(StatusCompleted)
	node2.SetStatus(StatusRunning)
	node3.SetStatus(StatusFailed)
	node3.SetError(assert.AnError)
	
	// Add execution times
	startTime := time.Now().Add(-10 * time.Minute)
	endTime := time.Now().Add(-5 * time.Minute)
	node1.SetStartTime(&startTime)
	node1.SetEndTime(&endTime)
	
	runningStart := time.Now().Add(-3 * time.Minute)
	node2.SetStartTime(&runningStart)
	
	failedStart := time.Now().Add(-2 * time.Minute)
	failedEnd := time.Now().Add(-1 * time.Minute)
	node3.SetStartTime(&failedStart)
	node3.SetEndTime(&failedEnd)
	
	// Add nodes to DAG
	err := dag.AddNode(node1)
	require.NoError(t, err)
	err = dag.AddNode(node2)
	require.NoError(t, err)
	err = dag.AddNode(node3)
	require.NoError(t, err)
	
	viz := NewDAGVisualization(dag)
	summary, err := viz.GenerateTextSummary()
	
	assert.NoError(t, err)
	assert.NotEmpty(t, summary)
	
	// Check that summary contains expected sections
	assert.Contains(t, summary, "=== Migration DAG Execution Summary ===")
	assert.Contains(t, summary, "Overall Statistics:")
	assert.Contains(t, summary, "Total Nodes: 3")
	assert.Contains(t, summary, "Completed: 1")
	assert.Contains(t, summary, "Failed: 1")
	assert.Contains(t, summary, "Running: 1")
	assert.Contains(t, summary, "Progress: 33.3%")
	
	// Check that different status sections are present
	assert.Contains(t, summary, "Completed Nodes (1):")
	assert.Contains(t, summary, "- completed (CompletedTask)")
	assert.Contains(t, summary, "Running Nodes (1):")
	assert.Contains(t, summary, "- running (RunningTask)")
	assert.Contains(t, summary, "Failed Nodes (1):")
	assert.Contains(t, summary, "- failed (FailedTask)")
	assert.Contains(t, summary, "Error: assert.AnError")
}

func TestDAGVisualization_ExportToText(t *testing.T) {
	dag := NewDAG()
	task := &MockTask{id: "test-task", taskType: "TestTask", description: "Test task"}
	node := NewBaseNode(task)
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	viz := NewDAGVisualization(dag)
	
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "dag_test_*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	// Export to text
	err = viz.ExportToText(tmpFile.Name())
	assert.NoError(t, err)
	
	// Read and verify the text file
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	
	textContent := string(data)
	assert.Contains(t, textContent, "=== Migration DAG Execution Summary ===")
	assert.Contains(t, textContent, "Total Nodes: 1")
	assert.Contains(t, textContent, "test-task")
}

func TestDAGVisualization_WithErrorHandling(t *testing.T) {
	dag := NewDAG()
	task := &MockTask{id: "failed-task", taskType: "FailedTask", description: "This task will fail"}
	node := NewBaseNode(task)
	
	err := dag.AddNode(node)
	require.NoError(t, err)
	
	// Set failed status with error
	node.SetStatus(StatusFailed)
	longError := strings.Repeat("This is a very long error message that should be truncated in DOT output. ", 10)
	node.SetError(fmt.Errorf(longError))
	
	viz := NewDAGVisualization(dag)
	
	// Test JSON export includes full error
	dagInfo, err := viz.GenerateDAGInfo()
	assert.NoError(t, err)
	assert.Len(t, dagInfo.Nodes, 1)
	assert.Contains(t, dagInfo.Nodes[0].Error, "This is a very long error message")
	
	// Test DOT export truncates long errors
	dot, err := viz.GenerateDOTGraph()
	assert.NoError(t, err)
	assert.Contains(t, dot, "Error:")
	// The error should be truncated in DOT output (but we can't easily test the exact truncation)
	
	// Test text summary includes error
	summary, err := viz.GenerateTextSummary()
	assert.NoError(t, err)
	assert.Contains(t, summary, "Failed Nodes (1):")
	assert.Contains(t, summary, "Error:")
}

func TestDAGVisualization_EmptyDAG(t *testing.T) {
	dag := NewDAG()
	viz := NewDAGVisualization(dag)
	
	dagInfo, err := viz.GenerateDAGInfo()
	assert.NoError(t, err)
	assert.NotNil(t, dagInfo)
	assert.Empty(t, dagInfo.Nodes)
	assert.Empty(t, dagInfo.Edges)
	assert.Equal(t, 0, dagInfo.Stats.TotalNodes)
	
	dot, err := viz.GenerateDOTGraph()
	assert.NoError(t, err)
	assert.Contains(t, dot, "digraph MigrationDAG")
	assert.Contains(t, dot, "Total: 0")
	
	summary, err := viz.GenerateTextSummary()
	assert.NoError(t, err)
	assert.Contains(t, summary, "Total Nodes: 0")
}

// MockTask for testing
type MockTask struct {
	id          string
	taskType    string
	description string
	executeFunc func(context.Context) (*TaskResult, error)
}

func (m *MockTask) GetID() string {
	return m.id
}

func (m *MockTask) GetName() string {
	return m.description
}

func (m *MockTask) GetType() string {
	return m.taskType
}

func (m *MockTask) GetDescription() string {
	return m.description
}

func (m *MockTask) Execute(ctx context.Context) (*TaskResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}
	result := NewTaskResult(m.GetID(), m.GetName())
	result.MarkStarted()
	result.MarkCompleted()
	return result, nil
}

func (m *MockTask) Validate() error {
	return nil
}