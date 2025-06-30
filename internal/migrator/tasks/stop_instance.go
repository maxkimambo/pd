package tasks

import (
	"context"
	"fmt"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// StopInstanceTask stops a running instance
type StopInstanceTask struct {
	BaseTask
}

// NewStopInstanceTask creates a new StopInstanceTask
func NewStopInstanceTask() taskmanager.TaskFunc {
	task := &StopInstanceTask{
		BaseTask: BaseTask{Name: "stop_instance"},
	}
	return task.Execute
}

// Execute stops the instance if it's running
func (t *StopInstanceTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// Check if instance is running
		instanceRunning, ok := shared.Get("instance_running")
		if !ok {
			return fmt.Errorf("instance_running not found in shared context")
		}
		
		isRunning, ok := instanceRunning.(bool)
		if !ok {
			return fmt.Errorf("invalid instance_running type in shared context")
		}
		
		// If instance is not running, skip this task
		if !isRunning {
			logger.User.Info("Instance is already stopped, skipping stop operation")
			shared.Set("instance_stopped", false)
			shared.Set("stop_operation_id", "")
			return nil
		}
		
		// Get instance details
		instanceName, ok := shared.Get("instance_name")
		if !ok {
			return fmt.Errorf("instance_name not found in shared context")
		}
		
		instanceZone, ok := shared.Get("instance_zone")
		if !ok {
			return fmt.Errorf("instance_zone not found in shared context")
		}
		
		// Get GCP client
		gcpClient, err := getGCPClient(shared)
		if err != nil {
			return err
		}
		
		// Get project ID from config
		configData, err := getConfig(shared)
		if err != nil {
			return err
		}
		
		projectID := ""
		if config, ok := configData.(map[string]interface{}); ok {
			if p, ok := config["ProjectID"].(string); ok {
				projectID = p
			}
		}
		
		if projectID == "" {
			return fmt.Errorf("project ID not found in config")
		}
		
		logger.User.Infof("Stopping instance %s in zone %s", instanceName, instanceZone)
		
		// Stop the instance
		err = gcpClient.ComputeClient.StopInstance(ctx, projectID, instanceZone.(string), instanceName.(string))
		if err != nil {
			return fmt.Errorf("failed to stop instance %s: %w", instanceName, err)
		}
		
		// Store operation results
		shared.Set("instance_stopped", true)
		shared.Set("stop_operation_id", fmt.Sprintf("stop-%s-%d", instanceName, ctx.Value("request_id")))
		
		logger.User.Successf("Instance %s stopped successfully", instanceName)
		
		return nil
	})
}