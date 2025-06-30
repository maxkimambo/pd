package taskmanager

import (
	"context"
	"testing"
)

func TestWorkflowBuilder_ValidWorkflow(t *testing.T) {
	// Create a valid workflow with dependencies: A -> B -> C
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskC := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	workflow, err := builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddTask("C", taskC).
		AddDependency("B", "A").
		AddDependency("C", "B").
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if workflow == nil {
		t.Fatal("Expected workflow to be created")
	}

	if workflow.ID != "test-workflow" {
		t.Errorf("Expected workflow ID 'test-workflow', got '%s'", workflow.ID)
	}

	if len(workflow.Tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(workflow.Tasks))
	}

	// Verify dependencies are set correctly
	if len(workflow.Tasks["A"].DependsOn) != 0 {
		t.Errorf("Task A should have no dependencies, got %v", workflow.Tasks["A"].DependsOn)
	}

	if len(workflow.Tasks["B"].DependsOn) != 1 || workflow.Tasks["B"].DependsOn[0] != "A" {
		t.Errorf("Task B should depend on A, got %v", workflow.Tasks["B"].DependsOn)
	}

	if len(workflow.Tasks["C"].DependsOn) != 1 || workflow.Tasks["C"].DependsOn[0] != "B" {
		t.Errorf("Task C should depend on B, got %v", workflow.Tasks["C"].DependsOn)
	}
}

func TestWorkflowBuilder_NonExistentTaskDependency(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	_, err := builder.
		AddTask("A", taskA).
		AddDependency("A", "B"). // B doesn't exist
		Build()

	if err == nil {
		t.Fatal("Expected error for non-existent dependency")
	}

	expectedError := "task 'A' depends on non-existent task 'B'"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestWorkflowBuilder_SimpleCycle(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	_, err := builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddDependency("A", "B").
		AddDependency("B", "A"). // Creates cycle A -> B -> A
		Build()

	if err == nil {
		t.Fatal("Expected error for cyclical dependency")
	}

	// Check that the error mentions cycle or circular dependency
	if !containsSubstring(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got '%s'", err.Error())
	}
}

func TestWorkflowBuilder_ComplexCycle(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskC := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	_, err := builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddTask("C", taskC).
		AddDependency("A", "B").
		AddDependency("B", "C").
		AddDependency("C", "A"). // Creates cycle A -> B -> C -> A
		Build()

	if err == nil {
		t.Fatal("Expected error for cyclical dependency")
	}

	// Check that the error mentions cycle or circular dependency
	if !containsSubstring(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got '%s'", err.Error())
	}
}

func TestWorkflowBuilder_EmptyWorkflow(t *testing.T) {
	builder := NewWorkflowBuilder("empty-workflow")

	workflow, err := builder.Build()

	if err != nil {
		t.Fatalf("Expected no error for empty workflow, got: %v", err)
	}

	if workflow == nil {
		t.Fatal("Expected workflow to be created")
	}

	if len(workflow.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(workflow.Tasks))
	}
}

func TestWorkflowBuilder_MultipleDependencies(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskC := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskD := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	workflow, err := builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddTask("C", taskC).
		AddTask("D", taskD).
		AddDependency("D", "A").
		AddDependency("D", "B").
		AddDependency("D", "C"). // D depends on A, B, and C
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Find task D and verify its dependencies
	workflowTaskD, exists := workflow.Tasks["D"]
	if !exists {
		t.Fatal("Task D not found in workflow")
	}

	if len(workflowTaskD.DependsOn) != 3 {
		t.Errorf("Task D should have 3 dependencies, got %d", len(workflowTaskD.DependsOn))
	}

	// Check that all dependencies are present
	depSet := make(map[string]bool)
	for _, dep := range workflowTaskD.DependsOn {
		depSet[dep] = true
	}

	for _, expectedDep := range []string{"A", "B", "C"} {
		if !depSet[expectedDep] {
			t.Errorf("Task D missing dependency on %s", expectedDep)
		}
	}
}

func TestWorkflowBuilder_SelfDependency(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	_, err := builder.
		AddTask("A", taskA).
		AddDependency("A", "A"). // Self dependency creates a cycle
		Build()

	if err == nil {
		t.Fatal("Expected error for self dependency")
	}

	// Check that the error mentions cycle or circular dependency
	if !containsSubstring(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got '%s'", err.Error())
	}
}

func TestWorkflowBuilder_ShowOrder(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskC := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	// Build workflow with dependencies: A -> B -> C
	builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddTask("C", taskC).
		AddDependency("B", "A").
		AddDependency("C", "B")

	order, err := builder.ShowOrder()
	if err != nil {
		t.Fatalf("ShowOrder() returned error: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(order))
	}

	// A should come before B, B should come before C
	posA := findPosition(order, "A")
	posB := findPosition(order, "B")
	posC := findPosition(order, "C")

	if posA == -1 || posB == -1 || posC == -1 {
		t.Fatal("ShowOrder() did not include all tasks")
	}

	if posA > posB || posB > posC {
		t.Errorf("ShowOrder() returned incorrect order: %v", order)
	}
}

func TestWorkflowBuilder_ShowOrder_CycleError(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	// Create cycle: A -> B -> A
	builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddDependency("A", "B").
		AddDependency("B", "A")

	order, err := builder.ShowOrder()
	if err == nil {
		t.Error("ShowOrder() should have returned error for cycle")
	}
	if order != nil {
		t.Error("ShowOrder() should return nil result when cycle detected")
	}

	// Check that the error mentions circular dependency
	if !containsSubstring(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got '%s'", err.Error())
	}
}

func TestWorkflowBuilder_ShowOrder_ComplexCase(t *testing.T) {
	builder := NewWorkflowBuilder("test-workflow")

	taskA := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskB := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskC := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }
	taskD := func(ctx context.Context, sharedCtx *SharedContext) error { return nil }

	// Create complex dependencies:
	//   A
	//  / \
	// B   C
	//  \ /
	//   D
	builder.
		AddTask("A", taskA).
		AddTask("B", taskB).
		AddTask("C", taskC).
		AddTask("D", taskD).
		AddDependency("B", "A").
		AddDependency("C", "A").
		AddDependency("D", "B").
		AddDependency("D", "C")

	order, err := builder.ShowOrder()
	if err != nil {
		t.Fatalf("ShowOrder() returned error: %v", err)
	}

	if len(order) != 4 {
		t.Fatalf("Expected 4 tasks, got %d", len(order))
	}

	// Verify ordering constraints
	posA := findPosition(order, "A")
	posB := findPosition(order, "B")
	posC := findPosition(order, "C")
	posD := findPosition(order, "D")

	// A should come before B and C
	if posA > posB || posA > posC {
		t.Errorf("A should come before B and C: %v", order)
	}

	// B and C should come before D
	if posB > posD || posC > posD {
		t.Errorf("B and C should come before D: %v", order)
	}
}

func TestWorkflowBuilder_ShowOrder_EmptyWorkflow(t *testing.T) {
	builder := NewWorkflowBuilder("empty-workflow")

	order, err := builder.ShowOrder()
	if err != nil {
		t.Fatalf("ShowOrder() returned error for empty workflow: %v", err)
	}

	if len(order) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(order))
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
