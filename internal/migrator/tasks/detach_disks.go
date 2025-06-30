package tasks

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// DetachDisksTask detaches non-boot disks from the instance
type DetachDisksTask struct {
	BaseTask
}

// NewDetachDisksTask creates a new DetachDisksTask
func NewDetachDisksTask() taskmanager.TaskFunc {
	task := &DetachDisksTask{
		BaseTask: BaseTask{Name: "detach_disks"},
	}
	return task.Execute
}

// Execute detaches all non-boot disks from the instance
func (t *DetachDisksTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		// Get instance details
		instanceName, ok := shared.Get("instance_name")
		if !ok {
			return fmt.Errorf("instance_name not found in shared context")
		}
		
		instanceZone, ok := shared.Get("instance_zone")
		if !ok {
			return fmt.Errorf("instance_zone not found in shared context")
		}
		
		// Get disk list
		diskListData, ok := shared.Get("disk_list")
		if !ok {
			return fmt.Errorf("disk_list not found in shared context")
		}
		
		diskList, ok := diskListData.([]*computepb.AttachedDisk)
		if !ok {
			return fmt.Errorf("invalid disk_list type in shared context")
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
		
		logger.User.Infof("Detaching %d non-boot disk(s) from instance %s", len(diskList), instanceName)
		
		// Map to store device names and attachment info
		detachedDisks := make(map[string]map[string]interface{})
		
		// Detach each disk
		for _, attachedDisk := range diskList {
			diskName := attachedDisk.GetDeviceName()
			
			logger.User.Infof("Detaching disk %s from instance %s", diskName, instanceName)
			
			// Store attachment metadata before detaching
			detachedDisks[diskName] = map[string]interface{}{
				"device_name": diskName,
				"source":      attachedDisk.GetSource(),
				"mode":        attachedDisk.GetMode(),
				"interface":   attachedDisk.GetInterface(),
			}
			
			// Detach the disk
			err := gcpClient.ComputeClient.DetachDisk(ctx, projectID, instanceZone.(string), instanceName.(string), diskName)
			if err != nil {
				return fmt.Errorf("failed to detach disk %s: %w", diskName, err)
			}
			
			logger.User.Successf("Disk %s detached successfully", diskName)
		}
		
		// Store detached disks info in shared context
		shared.Set("detached_disks", detachedDisks)
		shared.Set("detach_status", "completed")
		
		logger.User.Successf("All disks detached successfully from instance %s", instanceName)
		
		return nil
	})
}