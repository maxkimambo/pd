package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxkimambo/pd/integration_tests/internal/terraform"
	"github.com/stretchr/testify/require"
)

func TestErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		t.Fatal("GCP_PROJECT_ID environment variable must be set")
	}

	pdBinary := filepath.Join("..", "pd")

	tests := []struct {
		name           string
		args           []string
		expectedError  string
		setupResources bool
	}{
		{
			name: "invalid_project_id",
			args: []string{"migrate", "disk",
				"--project", "invalid-project-12345",
				"--zone", "us-central1-a",
				"--target-disk-type", "pd-ssd",
				"--auto-approve",
			},
			expectedError:  "failed to initialize GCP clients",
			setupResources: false,
		},
		{
			name: "invalid_zone",
			args: []string{"migrate", "disk",
				"--project", projectID,
				"--zone", "invalid-zone-123",
				"--target-disk-type", "pd-ssd",
				"--auto-approve",
			},
			expectedError:  "invalid zone",
			setupResources: false,
		},
		{
			name: "invalid_disk_type",
			args: []string{"migrate", "disk",
				"--project", projectID,
				"--zone", "us-central1-a",
				"--target-disk-type", "invalid-disk-type",
				"--auto-approve",
			},
			expectedError:  "invalid disk type",
			setupResources: false,
		},
		{
			name: "non_existent_instance",
			args: []string{"migrate", "compute",
				"--project", projectID,
				"--zone", "us-central1-a",
				"--instances", "non-existent-instance-12345",
				"--target-disk-type", "pd-ssd",
				"--auto-approve",
			},
			expectedError:  "instance not found",
			setupResources: false,
		},
		{
			name: "zone_and_region_conflict",
			args: []string{"migrate", "disk",
				"--project", projectID,
				"--zone", "us-central1-a",
				"--region", "us-central1",
				"--target-disk-type", "pd-ssd",
				"--auto-approve",
			},
			expectedError:  "cannot specify both",
			setupResources: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			var cleanup func()
			if tt.setupResources {
				runID := fmt.Sprintf("test-err-%d", time.Now().UnixNano())
				
				workDir, err := terraform.CreateTestWorkspace("terraform/scenarios/disk_migration")
				require.NoError(t, err)
				defer os.RemoveAll(workDir)

				tf := terraform.New(workDir)

				tfVars := map[string]any{
					"resource_prefix": runID,
					"project_id":      projectID,
					"zone":            "us-central1-a",
					"region":          "us-central1",
				}

				cleanup = func() {
					destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 10*time.Minute)
					defer destroyCancel()
					
					if err := tf.Destroy(destroyCtx, tfVars); err != nil {
						t.Errorf("Failed to destroy test resources: %v", err)
					}
				}

				require.NoError(t, tf.Init(ctx))
				_, err = tf.Apply(ctx, tfVars)
				require.NoError(t, err)
			}

			if cleanup != nil {
				defer cleanup()
			}

			cmd := exec.CommandContext(ctx, pdBinary, tt.args...)
			output, err := cmd.CombinedOutput()
			
			require.Error(t, err, "Command should have failed")
			outputStr := string(output)
			t.Logf("Command output: %s", outputStr)
			
			require.True(t, 
				strings.Contains(strings.ToLower(outputStr), strings.ToLower(tt.expectedError)),
				"Expected error containing '%s' but got: %s", tt.expectedError, outputStr)
		})
	}
}

func TestConcurrentMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		t.Fatal("GCP_PROJECT_ID environment variable must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	runID := fmt.Sprintf("test-concurrent-%d", time.Now().UnixNano())
	
	workDir, err := terraform.CreateTestWorkspace("terraform/scenarios/disk_migration")
	require.NoError(t, err)
	defer os.RemoveAll(workDir)

	tf := terraform.New(workDir)

	tfVars := map[string]any{
		"resource_prefix": runID,
		"project_id":      projectID,
		"zone":            "us-central1-a",
		"region":          "us-central1",
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

	zonalDisks := outputs["zonal_disk_names"].([]any)
	require.GreaterOrEqual(t, len(zonalDisks), 2, "Need at least 2 disks for concurrent test")

	time.Sleep(10 * time.Second)

	pdBinary := filepath.Join("..", "pd")
	cmd := exec.CommandContext(ctx, pdBinary, "migrate", "disk",
		"--project", projectID,
		"--zone", "us-central1-a",
		"--target-disk-type", "pd-ssd",
		"--auto-approve",
		"--concurrency", "3",
		"--label", "resource_prefix=" + runID)
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	start := time.Now()
	require.NoError(t, cmd.Run(), "Concurrent migration command failed")
	duration := time.Since(start)

	t.Logf("Migration of %d disks completed in %v with concurrency=3", len(zonalDisks), duration)
	
	require.Less(t, duration, 10*time.Minute, "Concurrent migration took too long")
}