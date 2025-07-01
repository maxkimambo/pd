package tasks

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// CheckInstanceStateTask checks the current state of the instance
type CheckInstanceStateTask struct {
	BaseTask
}

// NewCheckInstanceStateTask creates a new CheckInstanceStateTask
func NewCheckInstanceStateTask() taskmanager.TaskFunc {
	task := &CheckInstanceStateTask{
		BaseTask: BaseTask{Name: "check_instance_state"},
	}
	return task.Execute
}

// Execute checks if the instance is running or stopped
func (t *CheckInstanceStateTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// Get instance from shared context
		instanceData, ok := shared.Get("instance")
		if !ok {
			return fmt.Errorf("instance not found in shared context")
		}
		
		instance, ok := instanceData.(*computepb.Instance)
		if !ok {
			return fmt.Errorf("invalid instance type in shared context")
		}
		
		// Get GCP client
		gcpClient, err := getGCPClient(shared)
		if err != nil {
			return err
		}
		
		logger.Infof("Checking instance state for: %s", *instance.Name)
		
		// Check if instance is running
		isRunning := gcpClient.ComputeClient.InstanceIsRunning(ctx, instance)
		
		// Determine state string
		var state string
		if isRunning {
			state = "RUNNING"
		} else {
			state = "STOPPED"
		}
		
		// Store state in shared context
		shared.Set("instance_running", isRunning)
		shared.Set("original_state", state)
		
		logger.Infof("Instance %s is currently: %s", *instance.Name, state)
		
		return nil
	})
}