package migrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/utils"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
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
	logrus.Info("--- Phase 2: Migration ---")
	if len(disksToMigrate) == 0 {
		logrus.Info("No disks to migrate.")
		return []MigrationResult{}, nil
	}

	var wg sync.WaitGroup
	resultsChan := make(chan MigrationResult, len(disksToMigrate))
	concurrencyLimit := config.Concurrency
	semaphore := make(chan struct{}, concurrencyLimit)

	logrus.Infof("Starting migration for %d disk(s) with concurrency limit %d...", len(disksToMigrate), concurrencyLimit)

	for _, disk := range disksToMigrate {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(d *computepb.Disk) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result := migrateSingleDisk(ctx, config, gcpClient, d)
			resultsChan <- result
		}(disk)
	}

	wg.Wait()
	close(resultsChan)

	allResults := make([]MigrationResult, 0, len(disksToMigrate))
	for res := range resultsChan {
		allResults = append(allResults, res)
	}

	logrus.Info("--- Migration Phase Complete ---")
	return allResults, nil
}

func migrateSingleDisk(ctx context.Context, config *Config, gcpClient *gcp.Clients, disk *computepb.Disk) MigrationResult {
	startTime := time.Now()
	diskName := disk.GetName()
	zone := "unknown-zone"
	if disk.Zone != nil {
		parts := strings.Split(disk.GetZone(), "/")
		zone = parts[len(parts)-1]
	}

	logFields := logrus.Fields{"disk": diskName, "zone": zone}
	logrus.WithFields(logFields).Info("Starting migration worker")

	result := MigrationResult{
		DiskName:     diskName,
		Zone:         zone,
		OriginalDisk: diskName,
		Status:       "Pending",
	}

	snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, time.Now().Unix())
	result.SnapshotName = snapshotName
	logFields["snapshot"] = snapshotName
	logrus.WithFields(logFields).Info("Creating snapshot...")

	kmsParams := config.PopulateKmsParams()
	err := gcpClient.CreateSnapshot(ctx, config.ProjectID, zone, diskName, snapshotName, kmsParams, disk.GetLabels())
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create snapshot: %v", err)
		logrus.WithFields(logFields).Error(errMsg)
		result.Status = "Failed: Snapshot Creation"
		result.ErrorMessage = errMsg
		labelErr := gcpClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error")
		if labelErr != nil {
			logrus.WithFields(logFields).Warnf("Failed to apply 'migration:error' label to disk %s: %v", diskName, labelErr)
		}
		result.Duration = time.Since(startTime)
		return result
	}
	logrus.WithFields(logFields).Info("Snapshot created successfully.")

	if config.RetainName {
		logrus.WithFields(logFields).Info("Deleting original disk (retainName=true)...")
		err = gcpClient.DeleteDisk(ctx, config.ProjectID, zone, diskName)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to delete original disk: %v", err)
			logrus.WithFields(logFields).Error(errMsg)
			result.Status = "Failed: Disk Deletion"
			result.ErrorMessage = errMsg
			logrus.WithFields(logFields).Warn("Attempting to cleanup snapshot due to disk deletion failure...")
			cleanupErr := gcpClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if cleanupErr != nil {
				logrus.WithFields(logFields).Warnf("Failed to cleanup snapshot %s: %v", snapshotName, cleanupErr)
				result.ErrorMessage += fmt.Sprintf("Snapshot cleanup failed: %v", cleanupErr)
			} else {
				logrus.WithFields(logFields).Info("Snapshot cleanup successful.")
				result.SnapshotCleaned = true
			}
			result.Duration = time.Since(startTime)
			return result
		}
		logrus.WithFields(logFields).Info("Original disk deleted successfully.")
	} else {
		logrus.WithFields(logFields).Info("Skipping original disk deletion (retainName=false).")
	}

	newDiskName := diskName
	if !config.RetainName {
		newDiskName = utils.AddSuffix(diskName, 4)
		logrus.WithFields(logFields).Infof("Generated new disk name: %s", newDiskName)
	}
	result.NewDiskName = newDiskName
	logFields["newDisk"] = newDiskName

	logrus.WithFields(logFields).Infof("Recreating disk as type '%s'...", config.TargetDiskType)

	newDiskLabels := disk.GetLabels()
	if newDiskLabels == nil {
		newDiskLabels = make(map[string]string)
	}
	newDiskLabels["migration"] = "success"
	storagePoolUrl := utils.GetStoragePoolURL(config.storagePoolId, config.ProjectID, zone)
	err = gcpClient.CreateNewDiskFromSnapshot(ctx, config.ProjectID, zone, newDiskName, config.TargetDiskType, snapshotName, newDiskLabels, config.iops, config.throughput, storagePoolUrl)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to recreate disk from snapshot: %v", err)
		logrus.WithFields(logFields).Error(errMsg)
		result.Status = "Failed: Disk Recreation"
		result.ErrorMessage = errMsg
		if !config.RetainName {
			labelErr := gcpClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error-recreation-failed")
			if labelErr != nil {
				logrus.WithFields(logFields).Warnf("Failed to apply error label to original disk %s: %v", diskName, labelErr)
			}
		}
		logrus.WithFields(logFields).Warnf("Snapshot %s was not cleaned up and needs manual attention or cleanup phase.", snapshotName)
		result.Duration = time.Since(startTime)
		return result
	}
	logrus.WithFields(logFields).Info("Disk recreated successfully.")

	result.Status = "Success"
	result.Duration = time.Since(startTime)
	logrus.WithFields(logFields).Infof("Migration successful in %v", result.Duration)
	return result
}
