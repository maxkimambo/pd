package taskmanager

import (
	"fmt"
)

// WorkflowBuilder is a builder for creating Workflow instances with validation
type WorkflowBuilder struct {
	workflowID   string
	tasks        map[string]*Task
	dependencies map[string][]string // taskID -> list of dependency IDs
}

// NewWorkflowBuilder creates a new WorkflowBuilder with the given workflow ID
func NewWorkflowBuilder(id string) *WorkflowBuilder {
	return &WorkflowBuilder{
		workflowID:   id,
		tasks:        make(map[string]*Task),
		dependencies: make(map[string][]string),
	}
}

// AddTask adds a task to the workflow being built
func (wb *WorkflowBuilder) AddTask(id string, handler TaskFunc) *WorkflowBuilder {
	wb.tasks[id] = &Task{
		ID:      id,
		Handler: handler,
	}
	return wb
}

// AddDependency defines a dependency between two tasks
func (wb *WorkflowBuilder) AddDependency(taskID string, dependencyID string) *WorkflowBuilder {
	if wb.dependencies[taskID] == nil {
		wb.dependencies[taskID] = make([]string, 0)
	}
	wb.dependencies[taskID] = append(wb.dependencies[taskID], dependencyID)
	return wb
}

// Build validates and constructs the final Workflow object
func (wb *WorkflowBuilder) Build() (*Workflow, error) {
	// Validate all dependencies exist
	if err := wb.validateDependencies(); err != nil {
		return nil, err
	}

	// Create DAG and validate structure
	dag := wb.createDAG()
	if _, err := dag.TopologicalSort(); err != nil {
		return nil, fmt.Errorf("invalid workflow structure: %w", err)
	}

	// Set dependencies on tasks
	for taskID, deps := range wb.dependencies {
		if task, exists := wb.tasks[taskID]; exists {
			task.DependsOn = deps
		}
	}

	// Create workflow with all tasks
	return &Workflow{
		ID:    wb.workflowID,
		Tasks: wb.tasks,
	}, nil
}

// validateDependencies ensures all dependencies reference existing tasks
func (wb *WorkflowBuilder) validateDependencies() error {
	for taskID, deps := range wb.dependencies {
		for _, depID := range deps {
			if _, exists := wb.tasks[depID]; !exists {
				return fmt.Errorf("task '%s' depends on non-existent task '%s'", taskID, depID)
			}
		}
	}
	return nil
}

// ShowOrder returns the planned execution order without building the full Workflow
func (wb *WorkflowBuilder) ShowOrder() ([]string, error) {
	// Validate all dependencies exist
	if err := wb.validateDependencies(); err != nil {
		return nil, err
	}

	// Create DAG and get topological order
	dag := wb.createDAG()
	return dag.TopologicalSort()
}

// createDAG creates a DAG from the builder's current state
func (wb *WorkflowBuilder) createDAG() *DAG {
	dag := NewDAG()
	
	// Add all tasks as nodes
	for taskID := range wb.tasks {
		dag.AddNode(taskID)
	}
	
	// Add all dependencies as edges
	for taskID, deps := range wb.dependencies {
		for _, depID := range deps {
			dag.AddEdge(taskID, depID)
		}
	}
	
	return dag
}
