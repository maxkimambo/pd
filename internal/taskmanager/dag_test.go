package taskmanager

import (
	"testing"
)

func TestDAG_NewDAG(t *testing.T) {
	dag := NewDAG()
	if dag == nil {
		t.Fatal("NewDAG() returned nil")
	}
	if dag.nodes == nil || dag.edges == nil || dag.inDegree == nil {
		t.Fatal("NewDAG() did not initialize internal maps")
	}
}

func TestDAG_AddNode(t *testing.T) {
	dag := NewDAG()
	
	// Add a node
	dag.AddNode("task1")
	
	if !dag.nodes["task1"] {
		t.Error("AddNode() did not add node to nodes map")
	}
	if dag.inDegree["task1"] != 0 {
		t.Error("AddNode() did not initialize in-degree to 0")
	}
	
	// Adding same node again should not change anything
	dag.AddNode("task1")
	if dag.inDegree["task1"] != 0 {
		t.Error("AddNode() changed in-degree when adding existing node")
	}
}

func TestDAG_AddEdge(t *testing.T) {
	dag := NewDAG()
	
	// Add edge between two nodes
	dag.AddEdge("task1", "task2")
	
	// Both nodes should exist
	if !dag.nodes["task1"] || !dag.nodes["task2"] {
		t.Error("AddEdge() did not create both nodes")
	}
	
	// task1 should depend on task2
	if len(dag.edges["task1"]) != 1 || dag.edges["task1"][0] != "task2" {
		t.Error("AddEdge() did not add dependency correctly")
	}
	
	// task1 should have in-degree 1, task2 should have in-degree 0
	if dag.inDegree["task1"] != 1 {
		t.Error("AddEdge() did not increment in-degree for dependent task")
	}
	if dag.inDegree["task2"] != 0 {
		t.Error("AddEdge() incorrectly changed in-degree for dependency")
	}
}

func TestDAG_TopologicalSort_SimpleCase(t *testing.T) {
	dag := NewDAG()
	
	// Create simple dependency: task1 -> task2 -> task3
	dag.AddEdge("task1", "task2")
	dag.AddEdge("task2", "task3")
	
	result, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() returned error: %v", err)
	}
	
	// Should have all 3 tasks
	if len(result) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(result))
	}
	
	// task3 should come before task2, task2 should come before task1
	task1Pos := findPosition(result, "task1")
	task2Pos := findPosition(result, "task2")
	task3Pos := findPosition(result, "task3")
	
	if task3Pos == -1 || task2Pos == -1 || task1Pos == -1 {
		t.Fatal("TopologicalSort() did not include all tasks")
	}
	
	if task3Pos > task2Pos || task2Pos > task1Pos {
		t.Errorf("TopologicalSort() returned incorrect order: %v", result)
	}
}

func TestDAG_TopologicalSort_NoEdges(t *testing.T) {
	dag := NewDAG()
	
	// Add independent nodes
	dag.AddNode("task1")
	dag.AddNode("task2")
	dag.AddNode("task3")
	
	result, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() returned error: %v", err)
	}
	
	if len(result) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(result))
	}
	
	// All tasks should be present
	for _, task := range []string{"task1", "task2", "task3"} {
		if findPosition(result, task) == -1 {
			t.Errorf("TopologicalSort() missing task: %s", task)
		}
	}
}

func TestDAG_TopologicalSort_CycleDetection(t *testing.T) {
	dag := NewDAG()
	
	// Create cycle: task1 -> task2 -> task3 -> task1
	dag.AddEdge("task1", "task2")
	dag.AddEdge("task2", "task3")
	dag.AddEdge("task3", "task1")
	
	result, err := dag.TopologicalSort()
	if err == nil {
		t.Error("TopologicalSort() should have detected cycle but didn't return error")
	}
	if result != nil {
		t.Error("TopologicalSort() should return nil result when cycle detected")
	}
}

func TestDAG_TopologicalSort_ComplexCase(t *testing.T) {
	dag := NewDAG()
	
	// Create more complex dependency graph:
	//   task4
	//   /   \
	// task1  task2
	//   \   /
	//   task3
	dag.AddEdge("task1", "task4")
	dag.AddEdge("task2", "task4")
	dag.AddEdge("task3", "task1")
	dag.AddEdge("task3", "task2")
	
	result, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() returned error: %v", err)
	}
	
	if len(result) != 4 {
		t.Fatalf("Expected 4 tasks, got %d", len(result))
	}
	
	// Verify ordering constraints
	task1Pos := findPosition(result, "task1")
	task2Pos := findPosition(result, "task2")
	task3Pos := findPosition(result, "task3")
	task4Pos := findPosition(result, "task4")
	
	// task4 should come before task1 and task2
	if task4Pos > task1Pos || task4Pos > task2Pos {
		t.Errorf("task4 should come before task1 and task2: %v", result)
	}
	
	// task1 and task2 should come before task3
	if task1Pos > task3Pos || task2Pos > task3Pos {
		t.Errorf("task1 and task2 should come before task3: %v", result)
	}
}

func TestDAG_TopologicalSort_SelfCycle(t *testing.T) {
	dag := NewDAG()
	
	// Create self-cycle: task1 -> task1
	dag.AddEdge("task1", "task1")
	
	result, err := dag.TopologicalSort()
	if err == nil {
		t.Error("TopologicalSort() should have detected self-cycle but didn't return error")
	}
	if result != nil {
		t.Error("TopologicalSort() should return nil result when self-cycle detected")
	}
}

// Helper function to find position of task in result slice
func findPosition(slice []string, task string) int {
	for i, v := range slice {
		if v == task {
			return i
		}
	}
	return -1
}