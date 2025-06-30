package taskmanager

import (
	"context"
	"fmt"
)

// Workflow represents a collection of tasks and their dependencies.
type Workflow struct {
	ID    string
	Tasks map[string]*Task
}

// Execute runs the tasks in the workflow in correct topological order.
func (w *Workflow) Execute(ctx context.Context, sharedCtx *SharedContext) error {
	// Validate dependencies exist
	if err := w.validateDependencies(); err != nil {
		return err
	}
	
	// Create DAG from current workflow state
	dag := w.createDAG()
	
	// Perform topological sort to determine execution order
	executionOrder, err := dag.TopologicalSort()
	if err != nil {
		return fmt.Errorf("failed to determine execution order: %w", err)
	}

	// Execute tasks in order
	for _, taskID := range executionOrder {
		task := w.Tasks[taskID]
		if err := task.Handler(ctx, sharedCtx); err != nil {
			return fmt.Errorf("task %s failed: %w", taskID, err)
		}
	}

	return nil
}

// validateDependencies ensures all dependencies reference existing tasks
func (w *Workflow) validateDependencies() error {
	for _, task := range w.Tasks {
		for _, depID := range task.DependsOn {
			if _, exists := w.Tasks[depID]; !exists {
				return fmt.Errorf("task %s depends on non-existent task %s", task.ID, depID)
			}
		}
	}
	return nil
}

// createDAG creates a DAG from the workflow's current state
func (w *Workflow) createDAG() *DAG {
	dag := NewDAG()
	
	// Add all tasks as nodes
	for taskID := range w.Tasks {
		dag.AddNode(taskID)
	}
	
	// Add all dependencies as edges
	for _, task := range w.Tasks {
		for _, depID := range task.DependsOn {
			dag.AddEdge(task.ID, depID)
		}
	}
	
	return dag
}
