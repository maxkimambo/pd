package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxkimambo/pd/integration_tests/internal/gcloud"
	"github.com/maxkimambo/pd/integration_tests/internal/terraform"
	"github.com/stretchr/testify/require"
)

func TestDiskMigration(t *testing.T) {
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
		region           string
		targetDiskType   string
		expectedDiskType string
		sourceDiskType   string
		testRegional     bool
	}{
		{
			name:             "migrate_zonal_pd_balanced_to_hyperdisk_balanced",
			zone:             "us-central1-a",
			region:           "us-central1",
			targetDiskType:   "hyperdisk-balanced",
			expectedDiskType: "hyperdisk-balanced",
			sourceDiskType:   "pd-balanced",
			testRegional:     false,
		},
		{
			name:             "migrate_zonal_pd_ssd_to_hyperdisk_balanced",
			zone:             "us-central1-a",
			region:           "us-central1",
			targetDiskType:   "hyperdisk-balanced",
			expectedDiskType: "hyperdisk-balanced",
			sourceDiskType:   "pd-ssd",
			testRegional:     false,
		},
		{
			name:             "migrate_zonal_pd_standard_to_pd_balanced",
			zone:             "us-central1-a",
			region:           "us-central1",
			targetDiskType:   "pd-balanced",
			expectedDiskType: "pd-balanced",
			sourceDiskType:   "pd-standard",
			testRegional:     false,
		},
		{
			name:             "migrate_regional_pd_standard_to_pd_ssd",
			zone:             "us-central1-a",
			region:           "us-central1",
			targetDiskType:   "pd-ssd",
			expectedDiskType: "pd-ssd",
			sourceDiskType:   "pd-standard",
			testRegional:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			runID := fmt.Sprintf("test-%d", time.Now().UnixNano())
			
			workDir, err := terraform.CreateTestWorkspace("terraform/scenarios/disk_migration")
			require.NoError(t, err)
			defer os.RemoveAll(workDir)

			tf := terraform.New(workDir)
			gcClient := gcloud.NewClient(projectID)

			tfVars := map[string]any{
				"resource_prefix": runID,
				"project_id":      projectID,
				"zone":            tt.zone,
				"region":          tt.region,
				"disk_type":       tt.sourceDiskType,
			}

			t.Cleanup(func() {
				destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer destroyCancel()
				
				if err := tf.Destroy(destroyCtx, tfVars); err != nil {
					t.Errorf("Failed to destroy test resources: %v", err)
				}
			})

			require.NoError(t, tf.Init(ctx))

			outputs, err := tf.Apply(ctx, tfVars)
			require.NoError(t, err)

			var diskNames []string
			if tt.testRegional {
				regionalDisks := outputs["regional_disk_names"].([]any)
				for _, d := range regionalDisks {
					diskNames = append(diskNames, d.(string))
				}
			} else {
				zonalDisks := outputs["zonal_disk_names"].([]any)
				for _, d := range zonalDisks {
					diskNames = append(diskNames, d.(string))
				}
			}

			t.Logf("Created disks: %v", diskNames)

			time.Sleep(10 * time.Second)

			pdBinary := filepath.Join("..", "pd")
			args := []string{"migrate", "disk",
				"--project", projectID,
				"--target-disk-type", tt.targetDiskType,
				"--auto-approve",
				"--concurrency", "2",
				"--label", "resource_prefix=" + runID,
			}

			if tt.testRegional {
				args = append(args, "--region", tt.region)
			} else {
				args = append(args, "--zone", tt.zone)
			}

			cmd := exec.CommandContext(ctx, pdBinary, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			
			require.NoError(t, cmd.Run(), "Migration command failed")

			time.Sleep(10 * time.Second)

			for _, diskName := range diskNames {
				var diskInfo *gcloud.DiskInfo
				var err error
				
				if tt.testRegional {
					diskInfo, err = gcClient.GetRegionalDisk(ctx, tt.region, diskName)
				} else {
					diskInfo, err = gcClient.GetDisk(ctx, tt.zone, diskName)
				}
				
				require.NoError(t, err)
				
				var expectedType string
				if tt.testRegional {
					expectedType = fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s/diskTypes/%s",
						projectID, tt.region, tt.expectedDiskType)
				} else {
					expectedType = fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s",
						projectID, tt.zone, tt.expectedDiskType)
				}
				
				require.Equal(t, expectedType, diskInfo.Type,
					"Disk %s should have been migrated to %s", diskName, tt.expectedDiskType)
			}
		})
	}
}