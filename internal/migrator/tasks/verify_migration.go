package tasks

import (
	"context"
	"fmt"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// VerifyMigrationTask verifies that the migration was successful
type VerifyMigrationTask struct {
	BaseTask
}

// NewVerifyMigrationTask creates a new VerifyMigrationTask
func NewVerifyMigrationTask() taskmanager.TaskFunc {
	task := &VerifyMigrationTask{
		BaseTask: BaseTask{Name: "verify_migration"},
	}
	return task.Execute
}

// Execute verifies the migration was successful
func (t *VerifyMigrationTask) Execute(ctx context.Context, shared *taskmanager.SharedContext) error {
	return t.BaseTask.Execute(ctx, shared, func() error {
		logger.User.Info("Verifying migration success...")
		
		// Get instance details
		instanceData, ok := shared.Get("instance")
		if !ok {
			return fmt.Errorf("instance not found in shared context")
		}
		
		instance, ok := instanceData.(*computepb.Instance)
		if !ok {
			return fmt.Errorf("invalid instance type in shared context")
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
		
		// Get config for expected values
		configData, err := getConfig(shared)
		if err != nil {
			return err
		}
		
		var projectID, targetDiskType string
		if config, ok := configData.(map[string]interface{}); ok {
			if p, ok := config["ProjectID"].(string); ok {
				projectID = p
			}
			if t, ok := config["TargetDiskType"].(string); ok {
				targetDiskType = t
			}
		}
		
		// Get original disk metadata
		diskMetadataData, ok := shared.Get("disk_metadata")
		if !ok {
			return fmt.Errorf("disk_metadata not found in shared context")
		}
		
		diskMetadata, ok := diskMetadataData.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid disk_metadata type in shared context")
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
		
		// Verify each migrated disk
		var verificationErrors []error
		zone := instanceZone.(string)
		
		for originalName, newName := range migratedDisks {
			logger.Op.Debugf("Verifying disk %s (now %s)", originalName, newName)
			
			// Get the new disk
			disk, err := gcpClient.DiskClient.GetDisk(ctx, projectID, zone, newName)
			if err != nil {
				verificationErrors = append(verificationErrors, fmt.Errorf("failed to get disk %s: %w", newName, err))
				continue
			}
			
			if disk == nil {
				verificationErrors = append(verificationErrors, fmt.Errorf("disk %s not found", newName))
				continue
			}
			
			// Verify disk type
			actualType := extractDiskType(disk.GetType())
			if actualType != targetDiskType {
				verificationErrors = append(verificationErrors, 
					fmt.Errorf("disk %s has incorrect type: expected %s, got %s", newName, targetDiskType, actualType))
			}
			
			// Verify disk size matches original
			if originalMeta, ok := diskMetadata[originalName].(map[string]interface{}); ok {
				if originalSize, ok := originalMeta["disk_size_gb"].(int64); ok {
					if disk.GetSizeGb() != originalSize {
						verificationErrors = append(verificationErrors,
							fmt.Errorf("disk %s size mismatch: expected %d GB, got %d GB", 
								newName, originalSize, disk.GetSizeGb()))
					}
				}
			}
		}
		
		// Get the latest instance state to verify disks are attached
		updatedInstance, err := gcpClient.ComputeClient.GetInstance(ctx, projectID, zone, *instance.Name)
		if err != nil {
			verificationErrors = append(verificationErrors, 
				fmt.Errorf("failed to get updated instance state: %w", err))
		} else {
			// Count attached disks
			attachedCount := 0
			for _, attachedDisk := range updatedInstance.GetDisks() {
				if !attachedDisk.GetBoot() {
					attachedCount++
				}
			}
			
			expectedCount, _ := shared.Get("non_boot_disk_count")
			if attachedCount != expectedCount.(int) {
				verificationErrors = append(verificationErrors,
					fmt.Errorf("disk count mismatch: expected %d non-boot disks, found %d", 
						expectedCount, attachedCount))
			}
		}
		
		// Store verification results
		shared.Set("verification_errors", verificationErrors)
		
		if len(verificationErrors) > 0 {
			shared.Set("verification_status", "failed")
			logger.User.Warnf("Verification completed with %d issue(s)", len(verificationErrors))
			for _, err := range verificationErrors {
				logger.User.Warnf("  - %v", err)
			}
			return fmt.Errorf("verification failed with %d errors", len(verificationErrors))
		}
		
		shared.Set("verification_status", "passed")
		logger.User.Success("Migration verification passed - all disks migrated successfully")
		
		return nil
	})
}

// extractDiskType extracts the disk type from the full disk type URL
func extractDiskType(fullType string) string {
	// Example: https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/diskTypes/pd-ssd
	// We want just "pd-ssd"
	if idx := len(fullType) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if fullType[i] == '/' {
				return fullType[i+1:]
			}
		}
	}
	return fullType
}