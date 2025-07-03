package migrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

type MigrationResult struct {
	DiskName        string
	Zone            string
	Status          string
	Duration        time.Duration
	SnapshotName    string
	NewDiskName     string
	OriginalDisk    string
	ErrorMessage    string
	SnapshotCleaned bool
}

func MigrateDisks(ctx context.Context, config *Config, gcpClient *gcp.Clients, disksToMigrate []*computepb.Disk) ([]MigrationResult, error) {
	if config.DryRun {
		logger.Starting("[DRY-RUN] Starting disk migration simulation...")
	} else {
		logger.Starting("Starting disk migration...")
	}
	if len(disksToMigrate) == 0 {
		logger.Info("No disks to migrate")
		return []MigrationResult{}, nil
	}

	var wg sync.WaitGroup
	resultsChan := make(chan MigrationResult, len(disksToMigrate))
	concurrencyLimit := config.Concurrency
	semaphore := make(chan struct{}, concurrencyLimit)

	logger.Infof("Migrating %d disks (concurrency: %d)", len(disksToMigrate), concurrencyLimit)
	logger.WithFieldsMap(map[string]interface{}{
		"count":       len(disksToMigrate),
		"concurrency": concurrencyLimit,
	}).Info("migration phase started")

	for _, disk := range disksToMigrate {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(d *computepb.Disk) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result := MigrateSingleDisk(ctx, config, gcpClient, d)
			resultsChan <- result
		}(disk)
	}

	wg.Wait()
	close(resultsChan)

	allResults := make([]MigrationResult, 0, len(disksToMigrate))
	for res := range resultsChan {
		allResults = append(allResults, res)
	}

	logger.Success("Migration phase complete")
	logger.WithFieldsMap(map[string]interface{}{
		"total_disks": len(allResults),
	}).Info("migration phase completed")
	return allResults, nil
}

func MigrateSingleDisk(ctx context.Context, config *Config, gcpClient *gcp.Clients, disk *computepb.Disk) MigrationResult {
	startTime := time.Now()
	diskName := disk.GetName()
	zone := "unknown-zone"
	if disk.Zone != nil {
		parts := strings.Split(disk.GetZone(), "/")
		zone = parts[len(parts)-1]
	}

	logger.WithFieldsMap(map[string]interface{}{
		"disk": diskName,
		"zone": zone,
	}).Debug("starting disk migration worker")

	result := MigrationResult{
		DiskName:     diskName,
		Zone:         zone,
		OriginalDisk: diskName,
		Status:       "Pending",
	}

	snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, time.Now().Unix())
	result.SnapshotName = snapshotName

	logger.Snapshotf("Creating snapshot for %s", diskName)
	logger.WithFieldsMap(map[string]interface{}{
		"disk":     diskName,
		"zone":     zone,
		"snapshot": snapshotName,
	}).Info("initiating snapshot creation")

	kmsParams := config.PopulateKmsParams()
	err := gcpClient.SnapshotClient.CreateSnapshot(ctx, config.ProjectID, zone, diskName, snapshotName, kmsParams, disk.GetLabels())
	if err != nil {
		if strings.Contains(err.Error(), "quota") {
			logger.Error(utils.QuotaExceededError("snapshots", zone))
		} else if strings.Contains(err.Error(), "permission") {
			logger.Error(utils.PermissionError("create snapshot", diskName))
		} else {
			logger.Error(utils.FormatError(utils.ErrorContext{
				Operation:  "create snapshot",
				Resource:   diskName,
				Reason:     err.Error(),
				Suggestion: "Check disk status and ensure it's not in use",
				Command:    fmt.Sprintf("gcloud compute disks describe %s --zone=%s", diskName, zone),
			}, err))
		}
		result.Status = "Failed: Snapshot Creation"
		result.ErrorMessage = fmt.Sprintf("Failed to create snapshot: %v", err)
		labelErr := gcpClient.DiskClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error")
		if labelErr != nil {
			logger.WithFieldsMap(map[string]interface{}{
				"disk":  diskName,
				"zone":  zone,
				"error": labelErr.Error(),
			}).Warn("failed to apply error label to disk")
		}
		result.Duration = time.Since(startTime)
		return result
	}
	logger.WithFieldsMap(map[string]interface{}{
		"disk":     diskName,
		"zone":     zone,
		"snapshot": snapshotName,
	}).Info("snapshot creation completed")

	if config.RetainName {
		logger.Deletef("Deleting original disk %s", diskName)
		logger.WithFieldsMap(map[string]interface{}{
			"disk":        diskName,
			"zone":        zone,
			"retain_name": true,
		}).Info("deleting original disk")

		err = gcpClient.DiskClient.DeleteDisk(ctx, config.ProjectID, zone, diskName)

		if err != nil {
			if strings.Contains(err.Error(), "resourceInUse") {
				logger.Error(utils.FormatError(utils.ErrorContext{
					Operation:  "delete disk",
					Resource:   diskName,
					Reason:     "Disk is still attached to an instance",
					Suggestion: "Detach the disk from all instances before migration",
					Command:    fmt.Sprintf("gcloud compute disks describe %s --zone=%s", diskName, zone),
				}, err))
			} else {
				logger.Error(utils.FormatError(utils.ErrorContext{
					Operation:  "delete disk",
					Resource:   diskName,
					Reason:     err.Error(),
					Suggestion: "Verify disk exists and you have delete permissions",
				}, err))
			}
			result.Status = "Failed: Disk Deletion"
			result.ErrorMessage = fmt.Sprintf("Failed to delete original disk: %v", err)
			logger.Cleanupf("Cleaning up snapshot %s", snapshotName)
			logger.WithFieldsMap(map[string]interface{}{
				"snapshot": snapshotName,
				"reason":   "disk_deletion_failed",
			}).Info("attempting snapshot cleanup")

			// TODO: decide if we really want to delete the snapshot here

			// cleanupErr := gcpClient.SnapshotClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			// if cleanupErr != nil {
			// 	logger.Op.WithFields(map[string]interface{}{
			// 		"snapshot": snapshotName,
			// 		"error":    cleanupErr.Error(),
			// 	}).Error("snapshot cleanup failed")
			// 	result.ErrorMessage += fmt.Sprintf("Snapshot cleanup failed: %v", cleanupErr)
			// } else {
			// 	logger.Op.WithFields(map[string]interface{}{
			// 		"snapshot": snapshotName,
			// 	}).Info("snapshot cleanup successful")
			// 	result.SnapshotCleaned = true
			// }
			// result.Duration = time.Since(startTime)
			return result
		}
		logger.WithFieldsMap(map[string]interface{}{
			"disk": diskName,
			"zone": zone,
		}).Info("original disk deleted successfully")
	} else {
		logger.WithFieldsMap(map[string]interface{}{
			"disk":        diskName,
			"zone":        zone,
			"retain_name": false,
		}).Debug("skipping original disk deletion")
	}

	newDiskName := diskName
	if !config.RetainName {
		newDiskName = utils.AddSuffix(diskName)
		logger.WithFieldsMap(map[string]interface{}{
			"original_disk": diskName,
			"new_disk":      newDiskName,
		}).Debug("generated new disk name")
	}
	result.NewDiskName = newDiskName

	logger.Createf("Creating %s disk %s", config.TargetDiskType, newDiskName)
	logger.WithFieldsMap(map[string]interface{}{
		"disk":      diskName,
		"new_disk":  newDiskName,
		"zone":      zone,
		"disk_type": config.TargetDiskType,
		"snapshot":  snapshotName,
	}).Info("initiating disk recreation")

	newDiskLabels := disk.GetLabels()
	if newDiskLabels == nil {
		newDiskLabels = make(map[string]string)
	}
	newDiskLabels["migration"] = "success"
	storagePoolUrl := utils.GetStoragePoolURL(config.StoragePoolId, config.ProjectID, zone)
	err = gcpClient.DiskClient.CreateNewDiskFromSnapshot(ctx, config.ProjectID, zone, newDiskName, config.TargetDiskType, snapshotName, newDiskLabels, *disk.SizeGb, config.Iops, config.Throughput, storagePoolUrl)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to recreate disk from snapshot: %v", err)
		logger.Errorf("%s recreation failed", newDiskName)
		logger.WithFieldsMap(map[string]interface{}{
			"disk":     diskName,
			"new_disk": newDiskName,
			"zone":     zone,
			"error":    err.Error(),
		}).Error("disk recreation failed")
		result.Status = "Failed: Disk Recreation"
		result.ErrorMessage = errMsg
		if !config.RetainName {
			labelErr := gcpClient.DiskClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error-recreation-failed")
			if labelErr != nil {
				logger.WithFieldsMap(map[string]interface{}{
					"disk":  diskName,
					"zone":  zone,
					"error": labelErr.Error(),
				}).Warn("failed to apply error label to original disk")
			}
		}
		logger.WithFieldsMap(map[string]interface{}{
			"snapshot":      snapshotName,
			"action_needed": "manual_cleanup",
		}).Warn("snapshot requires manual cleanup")
		result.Duration = time.Since(startTime)
		return result
	}

	logger.Successf("%s migrated successfully", diskName)
	logger.WithFieldsMap(map[string]interface{}{
		"disk":     diskName,
		"new_disk": newDiskName,
		"zone":     zone,
		"duration": time.Since(startTime).String(),
	}).Info("disk migration completed successfully")

	result.Status = "Success"
	result.Duration = time.Since(startTime)
	return result
}
