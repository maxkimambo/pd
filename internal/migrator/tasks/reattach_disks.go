package tasks

import (
	"context"
	"fmt"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// ReattachDisksTask reattaches the migrated disks to the instance
type ReattachDisksTask struct {
	BaseTask
}

// NewReattachDisksTask creates a new ReattachDisksTask
func NewReattachDisksTask() taskmanager.TaskFunc {
	task := &ReattachDisksTask{
		BaseTask: BaseTask{Name: "reattach_disks"},
	}
	return task.Execute
}

// Execute reattaches the migrated disks to the instance
func (t *ReattachDisksTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
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
		
		// Get migrated disks mapping
		migratedDisksData, ok := shared.Get("migrated_disks")
		if !ok {
			return fmt.Errorf("migrated_disks not found in shared context")
		}
		
		migratedDisks, ok := migratedDisksData.(map[string]string)
		if !ok {
			return fmt.Errorf("invalid migrated_disks type in shared context")
		}
		
		// Get detached disks info for device names
		detachedDisksData, ok := shared.Get("detached_disks")
		if !ok {
			return fmt.Errorf("detached_disks not found in shared context")
		}
		
		detachedDisks, ok := detachedDisksData.(map[string]map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid detached_disks type in shared context")
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
		
		logger.User.Infof("Reattaching %d disk(s) to instance %s", len(migratedDisks), instanceName)
		
		// Track attached disks
		attachedDisks := make(map[string]string)
		
		// Reattach each disk
		for originalDiskName, newDiskName := range migratedDisks {
			// Get the original device name from detached disks info
			deviceName := originalDiskName // Default to disk name
			if diskInfo, ok := detachedDisks[originalDiskName]; ok {
				if dn, ok := diskInfo["device_name"].(string); ok {
					deviceName = dn
				}
			}
			
			logger.User.Infof("Attaching disk %s to instance %s as device %s", newDiskName, instanceName, deviceName)
			
			// Attach the disk
			err := gcpClient.ComputeClient.AttachDisk(
				ctx,
				projectID,
				instanceZone.(string),
				instanceName.(string),
				newDiskName,
				deviceName,
			)
			
			if err != nil {
				return fmt.Errorf("failed to attach disk %s: %w", newDiskName, err)
			}
			
			attachedDisks[deviceName] = newDiskName
			logger.User.Successf("Disk %s attached successfully", newDiskName)
		}
		
		// Store attachment info
		shared.Set("reattach_status", "completed")
		shared.Set("attached_disks", attachedDisks)
		
		logger.User.Successf("All disks reattached successfully to instance %s", instanceName)
		
		return nil
	})
}