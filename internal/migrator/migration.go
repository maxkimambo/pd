package migrator

import (
	"context"
	"fmt"
	"gcp-disk-migrator/internal/gcp"
	"math/rand" // For temporary unique suffix
	"strings"
	"sync"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sirupsen/logrus"
)

// MigrationResult holds the outcome of a migration attempt for a single disk.
type MigrationResult struct {
	DiskName        string
	Zone            string // Zone where the disk resided
	Status          string // e.g., "Success", "Failed: Snapshot Creation", "Failed: Disk Deletion", "Failed: Disk Recreation"
	Duration        time.Duration
	SnapshotName    string // Name of the snapshot created (even if cleanup fails later)
	NewDiskName     string // Name of the newly created disk
	OriginalDisk    string // Name of the original disk (useful if retainName=false)
	ErrorMessage    string // Detailed error message on failure
	SnapshotCleaned bool   // Indicates if the snapshot was successfully deleted in cleanup phase (will be set later)
}

// MigrateDisks performs Phase 2: concurrently migrating the discovered disks.
func MigrateDisks(ctx context.Context, config *Config, gcpClient *gcp.Clients, disksToMigrate []*computepb.Disk) ([]MigrationResult, error) {
	logrus.Info("--- Phase 2: Migration ---")
	if len(disksToMigrate) == 0 {
		logrus.Info("No disks to migrate.")
		return []MigrationResult{}, nil
	}

	var wg sync.WaitGroup
	resultsChan := make(chan MigrationResult, len(disksToMigrate))
	// Use a semaphore (channel) to limit concurrency
	concurrencyLimit := config.MaxConcurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 200 {
		logrus.Warnf("Invalid concurrency %d, defaulting to 10", concurrencyLimit)
		concurrencyLimit = 10
	}
	semaphore := make(chan struct{}, concurrencyLimit)

	logrus.Infof("Starting migration for %d disk(s) with concurrency limit %d...", len(disksToMigrate), concurrencyLimit)

	for _, disk := range disksToMigrate {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore slot

		go func(d *computepb.Disk) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore slot

			result := migrateSingleDisk(ctx, config, gcpClient, d)
			resultsChan <- result
		}(disk)
	}

	wg.Wait()
	close(resultsChan)

	// Collect results
	allResults := make([]MigrationResult, 0, len(disksToMigrate))
	for res := range resultsChan {
		allResults = append(allResults, res)
	}

	logrus.Info("--- Migration Phase Complete ---")
	return allResults, nil // Return collected results, error handling is within results
}

// migrateSingleDisk handles the migration steps for one disk.
func migrateSingleDisk(ctx context.Context, config *Config, gcpClient *gcp.Clients, disk *computepb.Disk) MigrationResult {
	startTime := time.Now()
	diskName := disk.GetName()
	// Extract zone from self-link
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
		OriginalDisk: diskName, // Store original name
		Status:       "Pending",
	}

	// --- Step 2.1: Create Snapshot ---
	snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, time.Now().Unix())
	result.SnapshotName = snapshotName // Record snapshot name early
	logFields["snapshot"] = snapshotName
	logrus.WithFields(logFields).Info("Step 2.1: Creating snapshot...")

	kmsParams := config.PopulateKmsParams() // Get KMS params if configured
	// Pass original disk labels to snapshot, CreateSnapshot adds 'managed-by'
	err := gcpClient.CreateSnapshot(ctx, config.ProjectID, zone, diskName, snapshotName, kmsParams, disk.GetLabels())
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create snapshot: %v", err)
		logrus.WithFields(logFields).Error(errMsg)
		result.Status = "Failed: Snapshot Creation"
		result.ErrorMessage = errMsg
		// Attempt to label the original disk as failed
		labelErr := gcpClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error") // Use new function name
		if labelErr != nil {
			logrus.WithFields(logFields).Warnf("Failed to apply 'migration:error' label to disk %s: %v", diskName, labelErr)
		}
		result.Duration = time.Since(startTime)
		return result
	}
	logrus.WithFields(logFields).Info("Step 2.1: Snapshot created successfully.")

	// --- Step 2.2: Handle Original Disk ---
	if config.RetainName {
		logrus.WithFields(logFields).Info("Step 2.2: Deleting original disk (retainName=true)...")
		err = gcpClient.DeleteDisk(ctx, config.ProjectID, zone, diskName)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to delete original disk: %v", err)
			logrus.WithFields(logFields).Error(errMsg)
			result.Status = "Failed: Disk Deletion"
			result.ErrorMessage = errMsg
			// Attempt cleanup snapshot created in step 1
			logrus.WithFields(logFields).Warn("Attempting to cleanup snapshot due to disk deletion failure...")
			cleanupErr := gcpClient.DeleteSnapshot(ctx, config.ProjectID, snapshotName)
			if cleanupErr != nil {
				logrus.WithFields(logFields).Warnf("Failed to cleanup snapshot %s: %v", snapshotName, cleanupErr)
				result.ErrorMessage += fmt.Sprintf(" | Snapshot cleanup failed: %v", cleanupErr)
			} else {
				logrus.WithFields(logFields).Info("Snapshot cleanup successful.")
				result.SnapshotCleaned = true // Mark as cleaned immediately if successful here
			}
			// Can't label disk as it failed deletion or is gone
			result.Duration = time.Since(startTime)
			return result
		}
		logrus.WithFields(logFields).Info("Step 2.2: Original disk deleted successfully.")
	} else {
		logrus.WithFields(logFields).Info("Step 2.2: Skipping original disk deletion (retainName=false).")
	}

	// --- Step 2.3: Recreate Disk from Snapshot ---
	newDiskName := diskName
	if !config.RetainName {
		// Generate a unique suffix - replace with a proper short UUID later
		suffix := fmt.Sprintf("%x", rand.Intn(0xFFF)) // Simple 3-char hex suffix
		newDiskName = fmt.Sprintf("%s-%s", diskName, suffix)
		logrus.WithFields(logFields).Infof("Generated new disk name: %s", newDiskName)
	}
	result.NewDiskName = newDiskName
	logFields["newDisk"] = newDiskName

	logrus.WithFields(logFields).Infof("Step 2.3: Recreating disk as type '%s'...", config.TargetDiskType)

	// Prepare labels for the new disk (original labels + success marker?)
	newDiskLabels := disk.GetLabels()
	if newDiskLabels == nil {
		newDiskLabels = make(map[string]string)
	}
	newDiskLabels["migration"] = "success"

	err = gcpClient.CreateNewDiskFromSnapshot(ctx, config.ProjectID, zone, newDiskName, config.TargetDiskType, snapshotName, newDiskLabels) // Use new function name
	if err != nil {
		errMsg := fmt.Sprintf("Failed to recreate disk from snapshot: %v", err)
		logrus.WithFields(logFields).Error(errMsg)
		result.Status = "Failed: Disk Recreation"
		result.ErrorMessage = errMsg
		// Disk creation failed. Original disk might be deleted (if retainName=true) or still exists (if retainName=false).
		// Snapshot definitely exists and needs cleanup later. Can't label the non-existent new disk.
		// If retainName=false, maybe label the original disk?
		if !config.RetainName {
			labelErr := gcpClient.UpdateDiskLabel(ctx, config.ProjectID, zone, diskName, "migration", "error-recreation-failed") // Use new function name
			if labelErr != nil {
				logrus.WithFields(logFields).Warnf("Failed to apply error label to original disk %s: %v", diskName, labelErr)
			}
		}
		logrus.WithFields(logFields).Warnf("Snapshot %s was not cleaned up and needs manual attention or cleanup phase.", snapshotName)
		result.Duration = time.Since(startTime)
		return result
	}
	logrus.WithFields(logFields).Info("Step 2.3: Disk recreated successfully.")

	// --- Migration Successful for this disk ---
	result.Status = "Success"
	result.Duration = time.Since(startTime)
	logrus.WithFields(logFields).Infof("Migration successful in %v", result.Duration)
	return result
}

// TODO: Replace random suffix generation with a proper short UUID library.
