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

func TestComputeMigration(t *testing.T) {
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
	}{
		{
			name:             "migrate_pd_balanced_to_hyperdisk_balanced_c3",
			zone:             "us-central1-a",
			targetDiskType:   "hyperdisk-balanced",
			expectedDiskType: "hyperdisk-balanced",
			sourceDiskType:   "pd-balanced",
			machineType:      "c3-standard-4",
			bootDiskType:     "pd-balanced",
		},
		{
			name:             "migrate_pd_ssd_to_hyperdisk_balanced_c3",
			zone:             "us-central1-a",
			targetDiskType:   "hyperdisk-balanced",
			expectedDiskType: "hyperdisk-balanced",
			sourceDiskType:   "pd-ssd",
			machineType:      "c3-standard-4",
			bootDiskType:     "pd-balanced",
		},
		{
			name:             "migrate_pd_standard_to_pd_ssd_n2",
			zone:             "us-central1-a",
			targetDiskType:   "pd-ssd",
			expectedDiskType: "pd-ssd",
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-8",
			bootDiskType:     "pd-standard",
		},
		{
			name:             "migrate_pd_standard_to_pd_balanced_n2",
			zone:             "us-central1-a",
			targetDiskType:   "pd-balanced",
			expectedDiskType: "pd-balanced",
			sourceDiskType:   "pd-standard",
			machineType:      "n2-standard-8",
			bootDiskType:     "pd-standard",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			runID := fmt.Sprintf("test-%d", time.Now().UnixNano())
			
			tfVars := map[string]any{
				"resource_prefix": runID,
				"project_id":      projectID,
				"zone":            tt.zone,
				"machine_type":    tt.machineType,
				"disk_type":       tt.sourceDiskType,
				"boot_disk_type":  tt.bootDiskType,
			}

			tf, cleanup := testutil.SetupTestWorkspace(t, "terraform/scenarios/compute_migration", tfVars)
			t.Cleanup(cleanup)
			gcClient := gcloud.NewClient(projectID)

			require.NoError(t, tf.Init(ctx))

			outputs, err := tf.Apply(ctx, tfVars)
			require.NoError(t, err)

			instanceName := outputs["instance_name"].(string)
			diskNames := outputs["attached_disk_names"].([]any)

			t.Logf("Created instance: %s with disks: %v", instanceName, diskNames)

			time.Sleep(30 * time.Second)

			pdBinary := testutil.GetPDBinaryPath()
			cmd := exec.CommandContext(ctx, pdBinary, "migrate", "compute",
				"--project", projectID,
				"--zone", tt.zone,
				"--instances", instanceName,
				"--target-disk-type", tt.targetDiskType,
				"--auto-approve",
				"--concurrency", "2")
			
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			
			require.NoError(t, cmd.Run(), "Migration command failed")

			time.Sleep(30 * time.Second)

			instance, err := gcClient.GetInstance(ctx, tt.zone, instanceName)
			require.NoError(t, err)
			require.Equal(t, "RUNNING", instance.Status)

			for _, disk := range instance.Disks {
				if disk.Type == "PERSISTENT" && disk.DeviceName != "persistent-disk-0" {
					diskName := gcloud.ExtractDiskNameFromSource(disk.Source)
					diskZone := gcloud.ExtractZoneFromPath(disk.Source)
					
					diskInfo, err := gcClient.GetDisk(ctx, diskZone, diskName)
					require.NoError(t, err)
					
					expectedType := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s",
						projectID, diskZone, tt.expectedDiskType)
					require.Equal(t, expectedType, diskInfo.Type,
						"Disk %s should have been migrated to %s", diskName, tt.expectedDiskType)
				}
			}
		})
	}
}