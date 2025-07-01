package tasks

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
	"github.com/maxkimambo/pd/internal/utils"
)

// ValidateInstanceTask validates that an instance exists and is ready for disk migration
type ValidateInstanceTask struct {
	BaseTask
	instance *computepb.Instance
}

// NewValidateInstanceTask creates a new ValidateInstanceTask
func NewValidateInstanceTask(instance *computepb.Instance) taskmanager.TaskFunc {
	task := &ValidateInstanceTask{
		BaseTask: BaseTask{Name: "validate_instance"},
		instance: instance,
	}
	return task.Execute
}

// Execute performs the instance validation
func (t *ValidateInstanceTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		if t.instance == nil {
			return fmt.Errorf("instance is nil")
		}
		
		if t.instance.Name == nil {
			return fmt.Errorf("instance name is nil")
		}
		
		if t.instance.Zone == nil {
			return fmt.Errorf("instance zone is nil")
		}
		
		logger.Infof("Validating instance: %s", *t.instance.Name)
		
		// Extract instance information
		instanceName := *t.instance.Name
		instanceZone := utils.ExtractZoneName(t.instance.GetZone())
		
		// Store instance information in shared context
		shared.Set("instance", t.instance)
		shared.Set("instance_name", instanceName)
		shared.Set("instance_zone", instanceZone)
		
		// Check if instance has disks
		attachedDisks := t.instance.GetDisks()
		if len(attachedDisks) == 0 {
			return fmt.Errorf("instance %s has no attached disks", instanceName)
		}
		
		// Identify non-boot disks
		var nonBootDisks []*computepb.AttachedDisk
		var bootDiskCount int
		
		for _, disk := range attachedDisks {
			if disk.GetBoot() {
				bootDiskCount++
				logger.Debugf("Found boot disk: %s", disk.GetDeviceName())
			} else {
				nonBootDisks = append(nonBootDisks, disk)
				logger.Debugf("Found non-boot disk: %s", disk.GetDeviceName())
			}
		}

		if len(nonBootDisks) == 0 {
			return fmt.Errorf("instance %s has no non-boot disks to migrate", instanceName)
		}
		
		// Store disk information in shared context
		shared.Set("disk_list", nonBootDisks)
		shared.Set("total_disks", len(attachedDisks))
		shared.Set("non_boot_disk_count", len(nonBootDisks))
		
		// Store disk metadata for verification later
		diskMetadata := make(map[string]interface{})
		for _, disk := range nonBootDisks {
			diskMetadata[disk.GetDeviceName()] = map[string]interface{}{
				"device_name":  disk.GetDeviceName(),
				"disk_source":  disk.GetSource(),
				"mode":         disk.GetMode(),
				"interface":    disk.GetInterface(),
				"disk_size_gb": disk.GetDiskSizeGb(),
			}
		}
		shared.Set("disk_metadata", diskMetadata)
		
		logger.Successf("Instance validation complete: %d non-boot disk(s) found for migration", len(nonBootDisks))
		shared.Set("validation_status", "completed")
		
		return nil
	})
}