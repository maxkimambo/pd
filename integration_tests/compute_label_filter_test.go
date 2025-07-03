package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/maxkimambo/pd/integration_tests/internal/gcloud"
	"github.com/maxkimambo/pd/integration_tests/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestComputeMigrationWithLabelFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		t.Fatal("GCP_PROJECT_ID environment variable must be set")
	}

	t.Parallel()

	tests := []struct {
		name             string
		zone             string
		targetDiskType   string
		expectedDiskType string
		sourceDiskType   string
		machineType      string
		bootDiskType     string
		labelFilter      string
		instanceLabels   map[string]string
		shouldMigrate    bool
		expectError      bool
	}{
		{
			name:             "migrate_instance_with_matching_label",
			zone:             "us-central1-a",
			targetDiskType:   "pd-ssd",
			expectedDiskType: "pd-ssd",
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-2",
			bootDiskType:     "pd-standard",
			labelFilter:      "environment=production",
			instanceLabels: map[string]string{
				"environment": "production",
				"app":         "web-server",
			},
			shouldMigrate: true,
			expectError:   false,
		},
		{
			name:             "skip_instance_with_non_matching_label",
			zone:             "us-central1-a",
			targetDiskType:   "pd-ssd",
			expectedDiskType: "pd-standard", // Should remain unchanged
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-2",
			bootDiskType:     "pd-standard",
			labelFilter:      "environment=production",
			instanceLabels: map[string]string{
				"environment": "development",
				"app":         "test-server",
			},
			shouldMigrate: false,
			expectError:   false,
		},
		{
			name:             "migrate_instance_with_label_presence_check",
			zone:             "us-central1-a",
			targetDiskType:   "pd-balanced",
			expectedDiskType: "pd-balanced",
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-2",
			bootDiskType:     "pd-standard",
			labelFilter:      "backup-enabled",
			instanceLabels: map[string]string{
				"backup-enabled": "true",
				"environment":    "staging",
			},
			shouldMigrate: true,
			expectError:   false,
		},
		{
			name:             "skip_instance_without_required_label",
			zone:             "us-central1-a",
			targetDiskType:   "pd-balanced",
			expectedDiskType: "pd-standard", // Should remain unchanged
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-2",
			bootDiskType:     "pd-standard",
			labelFilter:      "backup-enabled",
			instanceLabels: map[string]string{
				"environment": "testing",
				"app":         "test-app",
			},
			shouldMigrate: false,
			expectError:   false,
		},
		{
			name:             "migrate_hyperdisk_with_complex_label",
			zone:             "us-central1-a",
			targetDiskType:   "hyperdisk-balanced",
			expectedDiskType: "hyperdisk-balanced",
			sourceDiskType:   "pd-ssd",
			machineType:      "c3-standard-4",
			bootDiskType:     "pd-balanced",
			labelFilter:      "tier=premium gold",
			instanceLabels: map[string]string{
				"tier":        "premium gold",
				"environment": "production",
			},
			shouldMigrate: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			runID := fmt.Sprintf("label-test-%d", time.Now().UnixNano())

			// Create two instances - one with matching labels and one without
			tfVars := map[string]any{
				"resource_prefix":      runID,
				"project_id":           projectID,
				"zone":                 tt.zone,
				"machine_type":         tt.machineType,
				"disk_type":            tt.sourceDiskType,
				"boot_disk_type":       tt.bootDiskType,
				"random_data_size_mb":  100, // Small size for faster tests
				"create_test_instance": true,
				"test_instance_labels": tt.instanceLabels,
			}

			tf, cleanup := testutil.SetupTestWorkspace(t, "terraform/scenarios/compute_label_filter", tfVars)
			t.Cleanup(cleanup)
			gcClient := gcloud.NewClient(projectID)

			require.NoError(t, tf.Init(ctx))

			outputs, err := tf.Apply(ctx, tfVars)
			require.NoError(t, err)

			targetInstanceName := outputs["target_instance_name"].(string)
			decoyInstanceName := outputs["decoy_instance_name"].(string)

			t.Logf("Created target instance: %s with labels: %v", targetInstanceName, tt.instanceLabels)
			t.Logf("Created decoy instance: %s without matching labels", decoyInstanceName)

			// Wait for instances to be ready
			time.Sleep(30 * time.Second)

			// Run migration with label filter on all instances
			pdBinary := testutil.GetPDBinaryPath()
			cmd := exec.CommandContext(ctx, pdBinary, "migrate", "compute",
				"--project", projectID,
				"--zone", tt.zone,
				"--instances", "*", // Use wildcard to test filtering
				"--target-disk-type", tt.targetDiskType,
				"--label", tt.labelFilter,
				"--auto-approve",
				"--concurrency", "2")

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err = cmd.Run()
			if tt.expectError {
				require.Error(t, err, "Expected migration command to fail")
				return
			}
			
			if !tt.shouldMigrate {
				// When no instances match the filter, the command should fail with "no instances found"
				require.Error(t, err, "Expected migration command to fail when no instances match filter")
				t.Logf("Migration correctly failed with no matching instances")
			} else {
				require.NoError(t, err, "Migration command failed")
			}

			// Wait for operations to complete
			time.Sleep(30 * time.Second)

			// Verify target instance
			targetInstance, err := gcClient.GetInstance(ctx, tt.zone, targetInstanceName)
			require.NoError(t, err)
			require.Equal(t, "RUNNING", targetInstance.Status)

			// Check if target instance disks were migrated
			for _, disk := range targetInstance.Disks {
				if disk.Type == "PERSISTENT" && disk.DeviceName != "persistent-disk-0" {
					diskName := gcloud.ExtractDiskNameFromSource(disk.Source)
					diskZone := gcloud.ExtractZoneFromPath(disk.Source)

					diskInfo, err := gcClient.GetDisk(ctx, diskZone, diskName)
					require.NoError(t, err)

					if tt.shouldMigrate {
						expectedType := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s",
							projectID, diskZone, tt.expectedDiskType)
						require.Equal(t, expectedType, diskInfo.Type,
							"Target instance disk %s should have been migrated to %s", diskName, tt.expectedDiskType)
					} else {
						// Disk type should remain unchanged
						expectedType := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s",
							projectID, diskZone, tt.sourceDiskType)
						require.Equal(t, expectedType, diskInfo.Type,
							"Target instance disk %s should not have been migrated", diskName)
					}
				}
			}

			// Verify decoy instance was not touched
			decoyInstance, err := gcClient.GetInstance(ctx, tt.zone, decoyInstanceName)
			require.NoError(t, err)
			require.Equal(t, "RUNNING", decoyInstance.Status)

			// Check decoy instance disks remain unchanged
			for _, disk := range decoyInstance.Disks {
				if disk.Type == "PERSISTENT" && disk.DeviceName != "persistent-disk-0" {
					diskName := gcloud.ExtractDiskNameFromSource(disk.Source)
					diskZone := gcloud.ExtractZoneFromPath(disk.Source)

					diskInfo, err := gcClient.GetDisk(ctx, diskZone, diskName)
					require.NoError(t, err)

					expectedType := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s",
						projectID, diskZone, tt.sourceDiskType)
					require.Equal(t, expectedType, diskInfo.Type,
						"Decoy instance disk %s should not have been migrated", diskName)
				}
			}
		})
	}
}

func TestComputeMigrationWithSpecificInstanceAndLabelFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		t.Fatal("GCP_PROJECT_ID environment variable must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	runID := fmt.Sprintf("specific-label-test-%d", time.Now().UnixNano())

	// Create instance with specific labels
	tfVars := map[string]any{
		"resource_prefix":      runID,
		"project_id":           projectID,
		"zone":                 "us-central1-a",
		"machine_type":         "n2-standard-2",
		"disk_type":            "pd-standard",
		"boot_disk_type":       "pd-standard",
		"random_data_size_mb":  100,
		"create_test_instance": true,
		"test_instance_labels": map[string]string{
			"app":         "database",
			"environment": "production",
			"team":        "backend",
		},
	}

	tf, cleanup := testutil.SetupTestWorkspace(t, "terraform/scenarios/compute_label_filter", tfVars)
	t.Cleanup(cleanup)
	gcClient := gcloud.NewClient(projectID)

	require.NoError(t, tf.Init(ctx))

	outputs, err := tf.Apply(ctx, tfVars)
	require.NoError(t, err)

	targetInstanceName := outputs["target_instance_name"].(string)

	// Wait for instance to be ready
	time.Sleep(30 * time.Second)

	// Test 1: Specific instance name with matching label should succeed
	pdBinary := testutil.GetPDBinaryPath()
	cmd := exec.CommandContext(ctx, pdBinary, "migrate", "compute",
		"--project", projectID,
		"--zone", "us-central1-a",
		"--instances", targetInstanceName,
		"--target-disk-type", "pd-ssd",
		"--label", "app=database",
		"--auto-approve")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run(), "Migration with matching label should succeed")

	// Verify migration happened
	time.Sleep(30 * time.Second)
	instance, err := gcClient.GetInstance(ctx, "us-central1-a", targetInstanceName)
	require.NoError(t, err)

	for _, disk := range instance.Disks {
		if disk.Type == "PERSISTENT" && disk.DeviceName != "persistent-disk-0" {
			diskName := gcloud.ExtractDiskNameFromSource(disk.Source)
			diskZone := gcloud.ExtractZoneFromPath(disk.Source)

			diskInfo, err := gcClient.GetDisk(ctx, diskZone, diskName)
			require.NoError(t, err)

			expectedType := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/pd-ssd",
				projectID, diskZone)
			require.Equal(t, expectedType, diskInfo.Type,
				"Disk should have been migrated to pd-ssd")
		}
	}

	// Test 2: Same instance with non-matching label should fail
	cmd2 := exec.CommandContext(ctx, pdBinary, "migrate", "compute",
		"--project", projectID,
		"--zone", "us-central1-a",
		"--instances", targetInstanceName,
		"--target-disk-type", "pd-balanced",
		"--label", "app=web-server", // Non-matching label
		"--auto-approve")

	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr

	err = cmd2.Run()
	require.Error(t, err, "Migration with non-matching label should fail")
	t.Logf("Migration correctly failed for non-matching label")
}