package taskmanager

import (
	"fmt"
	"time"
)

type Workflow struct {
	ID          string
	Name        string
	Description string
	Tasks       []BaseTask
	dag         *DAG
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration // Total duration of the workflow execution
	Status      string        // e.g., "pending", "running", "completed", "failed"
}

func NewWorkflow(id, name, description string) *Workflow {
	dag := NewDAG()
	return &Workflow{
		ID:          id,
		Name:        name,
		Description: description,
		Tasks:       []BaseTask{},
		dag:         dag,
	}
}

func (w *Workflow) AddTask(task BaseTask) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}
	w.Tasks = append(w.Tasks, task)
	node := NewNode(task.ID(), task.Name(), "task", task)
	if err := w.dag.AddNode(node); err != nil {
		return fmt.Errorf("failed to add task to DAG: %w", err)
	}
	return nil
}

func (w *Workflow) AddDependency(fromTaskID, toTaskID string) error {

	if fromTaskID == "" || toTaskID == "" {
		return fmt.Errorf("task IDs cannot be empty")
	}

	if err := w.dag.AddDependency(fromTaskID, toTaskID); err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}
	return nil
}

func (w *Workflow) Execute() error {
	w.dag.Finalize() // Finalize the DAG to prevent further modifications
	if len(w.Tasks) == 0 {
		fmt.Println("No tasks to execute in the workflow")
		return nil
	}
	w.StartTime = time.Now()
	w.Status = "running"

	for {
		node, err := w.dag.PullNextTask()

		if err != nil {
			break
		}

		// Execute the task
		result, err := node.Execute()

		if err != nil {
			w.Status = "failed"
			return fmt.Errorf("workflow execution failed: %w", err)
		}

		// Handle the result (e.g., log it, update workflow status, etc.)
		fmt.Printf("Task %s completed with status: %s\n", node.ID, result.Status)
	}

	// if err := w.dag.Execute(); err != nil {
	// 	w.Status = "failed"
	// 	return fmt.Errorf("workflow execution failed: %w", err)
	// }

	w.EndTime = time.Now()
	w.Duration = w.EndTime.Sub(w.StartTime)
	w.Status = "completed"
	return nil
}
