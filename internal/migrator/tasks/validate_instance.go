package tasks

import (
	"context"
	"fmt"
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/taskmanager"
	"github.com/maxkimambo/pd/internal/utils"
	"github.com/maxkimambo/pd/internal/validation"
)

// ValidateInstanceTask validates that an instance exists and is ready for disk migration
type ValidateInstanceTask struct {
	BaseTask
	instance       *computepb.Instance
	targetDiskType string
	config         *migrator.Config
	gcpClient      *gcp.Clients
}

// NewValidateInstanceTask creates a new ValidateInstanceTask
func NewValidateInstanceTask(instance *computepb.Instance, targetDiskType string, config *migrator.Config, gcpClient *gcp.Clients) taskmanager.TaskFunc {
	task := &ValidateInstanceTask{
		BaseTask:       BaseTask{Name: "validate_instance"},
		instance:       instance,
		targetDiskType: targetDiskType,
		config:         config,
		gcpClient:      gcpClient,
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
		machineType := utils.ExtractMachineType(t.instance.GetMachineType())
		
		// Validate machine type compatibility with target disk type
		validationResult := validation.IsCompatible(machineType, t.targetDiskType)
		logger.Infof("Instance %s: Machine type %s validation against target disk type %s: %v", 
			instanceName, machineType, t.targetDiskType, validationResult)
		
		if !validationResult.Compatible {
			return fmt.Errorf("instance %s validation failed: %s", instanceName, validationResult.Reason)
		}
		
		// Store instance information in shared context
		shared.Set("instance", t.instance)
		shared.Set("instance_name", instanceName)
		shared.Set("instance_zone", instanceZone)
		
		// Check if instance has disks
		attachedDisks := t.instance.GetDisks()
		if len(attachedDisks) == 0 {
			return fmt.Errorf("instance %s has no attached disks", instanceName)
		}
		
		// Identify non-boot disks and validate disk types
		var nonBootDisks []*computepb.AttachedDisk
		var bootDiskCount int
		var diskFailureReasons []string
		
		for _, disk := range attachedDisks {
			if disk.GetBoot() {
				bootDiskCount++
				logger.Debugf("Found boot disk: %s", disk.GetDeviceName())
			} else {
				// Validate disk can be migrated to target type
				if disk.GetSource() != "" {
					diskName := ""
					parts := strings.Split(disk.GetSource(), "/")
					diskName = parts[len(parts)-1]
					
					if diskName != "" {
						// Get disk details
						diskDetails, err := t.gcpClient.DiskClient.GetDisk(ctx, t.config.ProjectID, instanceZone, diskName)
						if err != nil {
							logger.Debugf("Warning: Could not get disk details for %s: %v", diskName, err)
						} else {
							currentDiskType := utils.ExtractDiskType(diskDetails.GetType())
							
							// Check if disk is already the target type
							if currentDiskType == t.targetDiskType {
								logger.Debugf("Disk %s already has target type %s, skipping", diskName, t.targetDiskType)
								continue
							}
							
							// Check for incompatible disk types
							if currentDiskType == "local-ssd" {
								diskFailureReasons = append(diskFailureReasons, 
									fmt.Sprintf("disk %s has type local-ssd which cannot be migrated", diskName))
								continue
							}
						}
					}
				}
				
				nonBootDisks = append(nonBootDisks, disk)
				logger.Debugf("Found non-boot disk: %s", disk.GetDeviceName())
			}
		}
		
		// Check if any disks failed validation
		if len(diskFailureReasons) > 0 {
			return fmt.Errorf("disk validation failed: %s", strings.Join(diskFailureReasons, "; "))
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
		
		logger.Successf("Instance validation complete: %s with machine type %s is compatible with %s. %d non-boot disk(s) found for migration", 
			instanceName, machineType, t.targetDiskType, len(nonBootDisks))
		shared.Set("validation_status", "completed")
		shared.Set("machine_type", machineType)
		shared.Set("target_disk_type", t.targetDiskType)
		
		return nil
	})
}