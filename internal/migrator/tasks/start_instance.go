package tasks

import (
	"context"
	"fmt"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// StartInstanceTask starts an instance that was previously stopped
type StartInstanceTask struct {
	BaseTask
}

// NewStartInstanceTask creates a new StartInstanceTask
func NewStartInstanceTask() taskmanager.TaskFunc {
	task := &StartInstanceTask{
		BaseTask: BaseTask{Name: "start_instance"},
	}
	return task.Execute
}

// Execute starts the instance if it was originally running
func (t *StartInstanceTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// Check original state
		originalState, ok := shared.Get("original_state")
		if !ok {
			return fmt.Errorf("original_state not found in shared context")
		}

		// If instance was not running originally, skip
		if originalState != "RUNNING" {
			logger.Info("Instance was not originally running, skipping start operation")
			shared.Set("instance_started", false)
			shared.Set("start_operation_id", "")
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

		// Get project ID
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

		logger.Infof("Starting instance %s in zone %s", instanceName, instanceZone)

		// Start the instance
		err = gcpClient.ComputeClient.StartInstance(ctx, projectID, instanceZone.(string), instanceName.(string))
		if err != nil {
			return fmt.Errorf("failed to start instance %s: %w", instanceName, err)
		}

		// Store operation results
		shared.Set("instance_started", true)
		shared.Set("start_operation_id", fmt.Sprintf("start-%s-%d", instanceName, ctx.Value("request_id")))

		logger.Successf("Instance %s started successfully", instanceName)

		return nil
	})
}
