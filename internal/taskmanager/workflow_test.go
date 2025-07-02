package taskmanager

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkflow_Execute_SimpleChain(t *testing.T) {
	// Test case: A -> B -> C
	sharedCtx := NewSharedContext()
	var executionOrder []string

	// Create tasks that record their execution order
	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "A")
			sharedCtx.Set("A_executed", true)
			return nil
		},
		DependsOn: []string{},
	}

	taskB := &Task{
		ID: "B",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "B")
			sharedCtx.Set("B_executed", true)
			return nil
		},
		DependsOn: []string{"A"},
	}

	taskC := &Task{
		ID: "C",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "C")
			sharedCtx.Set("C_executed", true)
			return nil
		},
		DependsOn: []string{"B"},
	}

	workflow := &Workflow{
		ID: "test-chain",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
			"C": taskC,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify execution
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedOrder := []string{"A", "B", "C"}
	if !equalSlices(executionOrder, expectedOrder) {
		t.Errorf("Expected execution order %v, got %v", expectedOrder, executionOrder)
	}

	// Verify all tasks were executed
	for _, taskID := range []string{"A", "B", "C"} {
		if executed, ok := sharedCtx.Get(taskID + "_executed"); !ok || !executed.(bool) {
			t.Errorf("Task %s was not executed", taskID)
		}
	}
}

func TestWorkflow_Execute_DiamondDependency(t *testing.T) {
	// Test case: A -> B, A -> C, B -> D, C -> D
	sharedCtx := NewSharedContext()
	var executionOrder []string

	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "A")
			return nil
		},
		DependsOn: []string{},
	}

	taskB := &Task{
		ID: "B",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "B")
			return nil
		},
		DependsOn: []string{"A"},
	}

	taskC := &Task{
		ID: "C",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "C")
			return nil
		},
		DependsOn: []string{"A"},
	}

	taskD := &Task{
		ID: "D",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "D")
			return nil
		},
		DependsOn: []string{"B", "C"},
	}

	workflow := &Workflow{
		ID: "test-diamond",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
			"C": taskC,
			"D": taskD,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify execution
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify that A comes first, D comes last, and B/C come after A but before D
	if len(executionOrder) != 4 {
		t.Fatalf("Expected 4 tasks to be executed, got %d", len(executionOrder))
	}

	if executionOrder[0] != "A" {
		t.Errorf("Expected A to be executed first, got %s", executionOrder[0])
	}

	if executionOrder[3] != "D" {
		t.Errorf("Expected D to be executed last, got %s", executionOrder[3])
	}

	// B and C should come after A but before D
	bcIndices := []int{}
	for i, task := range executionOrder {
		if task == "B" || task == "C" {
			bcIndices = append(bcIndices, i)
		}
	}

	if len(bcIndices) != 2 {
		t.Errorf("Expected B and C to be executed, got indices %v", bcIndices)
	}

	for _, idx := range bcIndices {
		if idx <= 0 || idx >= 3 {
			t.Errorf("B or C executed at wrong position: %d", idx)
		}
	}
}

func TestWorkflow_Execute_ErrorPropagation(t *testing.T) {
	// Test case: A -> B -> C, where B fails
	sharedCtx := NewSharedContext()
	var executionOrder []string
	expectedError := errors.New("task B failed")

	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "A")
			return nil
		},
		DependsOn: []string{},
	}

	taskB := &Task{
		ID: "B",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "B")
			return expectedError
		},
		DependsOn: []string{"A"},
	}

	taskC := &Task{
		ID: "C",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "C")
			return nil
		},
		DependsOn: []string{"B"},
	}

	workflow := &Workflow{
		ID: "test-error",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
			"C": taskC,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify error handling
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "task B failed") {
		t.Errorf("Expected error to contain 'task B failed', got: %v", err)
	}

	// Verify execution stopped at B
	expectedOrder := []string{"A", "B"}
	if !equalSlices(executionOrder, expectedOrder) {
		t.Errorf("Expected execution order %v, got %v", expectedOrder, executionOrder)
	}

	// Verify C was not executed
	if len(executionOrder) > 2 {
		t.Errorf("Task C should not have been executed after B failed")
	}
}

func TestWorkflow_Execute_CircularDependency(t *testing.T) {
	// Test case: A -> B -> A (circular dependency)
	sharedCtx := NewSharedContext()

	taskA := &Task{
		ID:        "A",
		Handler:   func(ctx context.Context, sharedCtx *SharedContext) error { return nil },
		DependsOn: []string{"B"},
	}

	taskB := &Task{
		ID:        "B",
		Handler:   func(ctx context.Context, sharedCtx *SharedContext) error { return nil },
		DependsOn: []string{"A"},
	}

	workflow := &Workflow{
		ID: "test-circular",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify circular dependency is detected
	if err == nil {
		t.Fatal("Expected error for circular dependency, got nil")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Expected error to contain 'circular dependency', got: %v", err)
	}
}

func TestWorkflow_Execute_NonExistentDependency(t *testing.T) {
	// Test case: A depends on non-existent task X
	sharedCtx := NewSharedContext()

	taskA := &Task{
		ID:        "A",
		Handler:   func(ctx context.Context, sharedCtx *SharedContext) error { return nil },
		DependsOn: []string{"X"}, // X doesn't exist
	}

	workflow := &Workflow{
		ID: "test-missing-dep",
		Tasks: map[string]*Task{
			"A": taskA,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify missing dependency is detected
	if err == nil {
		t.Fatal("Expected error for missing dependency, got nil")
	}

	if !strings.Contains(err.Error(), "non-existent task") {
		t.Errorf("Expected error to contain 'non-existent task', got: %v", err)
	}
}

func TestWorkflow_Execute_EmptyWorkflow(t *testing.T) {
	// Test case: empty workflow
	sharedCtx := NewSharedContext()

	workflow := &Workflow{
		ID:    "test-empty",
		Tasks: map[string]*Task{},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify empty workflow succeeds
	if err != nil {
		t.Errorf("Expected no error for empty workflow, got: %v", err)
	}
}

func TestWorkflow_Execute_SingleTask(t *testing.T) {
	// Test case: single task with no dependencies
	sharedCtx := NewSharedContext()
	executed := false

	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executed = true
			return nil
		},
		DependsOn: []string{},
	}

	workflow := &Workflow{
		ID: "test-single",
		Tasks: map[string]*Task{
			"A": taskA,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify execution
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !executed {
		t.Error("Expected task A to be executed")
	}
}

func TestWorkflow_Execute_ParallelBranches(t *testing.T) {
	// Test case: A -> B, A -> C (B and C can run in parallel after A)
	sharedCtx := NewSharedContext()
	var executionOrder []string

	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "A")
			return nil
		},
		DependsOn: []string{},
	}

	taskB := &Task{
		ID: "B",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "B")
			return nil
		},
		DependsOn: []string{"A"},
	}

	taskC := &Task{
		ID: "C",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			executionOrder = append(executionOrder, "C")
			return nil
		},
		DependsOn: []string{"A"},
	}

	workflow := &Workflow{
		ID: "test-parallel",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
			"C": taskC,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify execution
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify A comes first
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 tasks to be executed, got %d", len(executionOrder))
	}

	if executionOrder[0] != "A" {
		t.Errorf("Expected A to be executed first, got %s", executionOrder[0])
	}

	// B and C should come after A (order between B and C doesn't matter)
	bcTasks := []string{executionOrder[1], executionOrder[2]}
	if !containsAll(bcTasks, []string{"B", "C"}) {
		t.Errorf("Expected B and C to be executed after A, got %v", bcTasks)
	}
}

func TestWorkflow_Execute_DataPassing(t *testing.T) {
	// Test case: A -> B, where A sets data and B uses it
	sharedCtx := NewSharedContext()

	taskA := &Task{
		ID: "A",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			sharedCtx.Set("result_from_A", "hello")
			return nil
		},
		DependsOn: []string{},
	}

	taskB := &Task{
		ID: "B",
		Handler: func(ctx context.Context, sharedCtx *SharedContext) error {
			value, exists := sharedCtx.Get("result_from_A")
			if !exists {
				t.Error("Expected result_from_A to exist in shared context")
				return errors.New("result_from_A not found")
			}

			if value != "hello" {
				t.Errorf("Expected result_from_A to be 'hello', got %v", value)
				return errors.New("unexpected value")
			}

			// Set another value to verify chaining
			sharedCtx.Set("result_from_B", "world")
			return nil
		},
		DependsOn: []string{"A"},
	}

	workflow := &Workflow{
		ID: "test-data-passing",
		Tasks: map[string]*Task{
			"A": taskA,
			"B": taskB,
		},
	}

	// Execute workflow
	ctx := context.Background()
	err := workflow.Execute(ctx, sharedCtx)

	// Verify execution
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify data was passed correctly
	valueA, existsA := sharedCtx.Get("result_from_A")
	if !existsA || valueA != "hello" {
		t.Errorf("Expected result_from_A to be 'hello', got %v (exists: %v)", valueA, existsA)
	}

	valueB, existsB := sharedCtx.Get("result_from_B")
	if !existsB || valueB != "world" {
		t.Errorf("Expected result_from_B to be 'world', got %v (exists: %v)", valueB, existsB)
	}
}

// Helper functions for testing

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsAll(slice, elements []string) bool {
	elementMap := make(map[string]bool)
	for _, elem := range elements {
		elementMap[elem] = false
	}

	for _, item := range slice {
		if _, exists := elementMap[item]; exists {
			elementMap[item] = true
		}
	}

	for _, found := range elementMap {
		if !found {
			return false
		}
	}
	return true
}
