package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// BaseTask provides common functionality for all tasks
type BaseTask struct {
	Name string
}

// Execute logs task start/end and measures execution time
func (b *BaseTask) Execute(ctx context.Context, shared *taskmanager.SharedContext, taskFunc func() error) error {
	startTime := time.Now()
	logger.Debugf("Starting task: %s", b.Name)
	
	err := taskFunc()
	
	duration := time.Since(startTime)
	shared.Set(b.Name+"_duration", duration)
	
	if err != nil {
		logger.Errorf("Task %s failed after %v: %v", b.Name, duration, err)
		return err
	}
	
	logger.Debugf("Task %s completed successfully in %v", b.Name, duration)
	return nil
}

// getGCPClient retrieves the GCP client from shared context
func getGCPClient(shared *taskmanager.SharedContext) (*gcp.Clients, error) {
	client, ok := shared.Get("gcp_client")
	if !ok {
		return nil, fmt.Errorf("GCP client not found in shared context")
	}
	
	gcpClient, ok := client.(*gcp.Clients)
	if !ok {
		return nil, fmt.Errorf("invalid GCP client type in shared context")
	}
	
	return gcpClient, nil
}

// getConfig retrieves the migration config from shared context
func getConfig(shared *taskmanager.SharedContext) (interface{}, error) {
	config, ok := shared.Get("config")
	if !ok {
		return nil, fmt.Errorf("config not found in shared context")
	}
	
	return config, nil
}