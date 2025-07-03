package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxkimambo/pd/integration_tests/internal/terraform"
	"github.com/stretchr/testify/require"
)

// SetupTestWorkspace creates a new test workspace for integration tests.
// It creates a unique directory in tmp_integration_tests/, copies the scenario files,
// and returns a terraform instance and cleanup function.
// The cleanup function respects PRESERVE_TF_RESOURCES for both terraform destroy and directory cleanup.
func SetupTestWorkspace(t *testing.T, scenarioPath string, tfVars map[string]any) (*terraform.Terraform, func()) {
	t.Helper()

	// Get project root (go up from integration_tests directory)
	projectRoot, err := filepath.Abs(filepath.Join("..", "tmp_integration_tests"))
	require.NoError(t, err, "failed to get project root path")

	// Create a unique name for the test workspace directory
	randomBytes := make([]byte, 4)
	_, err = rand.Read(randomBytes)
	require.NoError(t, err, "failed to generate random bytes")
	randomSuffix := hex.EncodeToString(randomBytes)
	
	testName := strings.ReplaceAll(t.Name(), "/", "_") // Sanitize test name for directory
	workspaceName := fmt.Sprintf("%s-%s", testName, randomSuffix)
	
	// Create the workspace directory inside tmp_integration_tests/
	workspaceDir := filepath.Join(projectRoot, workspaceName)
	err = os.MkdirAll(workspaceDir, 0755)
	require.NoError(t, err, "failed to create test workspace directory")

	// Determine the actual terraform scenario path within the workspace
	scenarioName := filepath.Base(scenarioPath)
	scenarioType := filepath.Base(filepath.Dir(scenarioPath))
	actualWorkDir := filepath.Join(workspaceDir, scenarioType, scenarioName)

	// Copy the terraform scenario to the new workspace
	err = terraform.CreateTestWorkspace(scenarioPath, workspaceDir)
	require.NoError(t, err, "failed to copy terraform scenario to workspace")

	tf := terraform.New(actualWorkDir)
	
	// Create cleanup function that handles both terraform destroy and directory cleanup
	cleanup := func() {
		preserveResources := os.Getenv("PRESERVE_TF_RESOURCES") == "true"
		
		if !preserveResources {
			// Destroy terraform resources
			destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer destroyCancel()
			
			if err := tf.Destroy(destroyCtx, tfVars); err != nil {
				t.Errorf("Failed to destroy test resources: %v", err)
			}
			
			// Remove the workspace directory
			if err := os.RemoveAll(workspaceDir); err != nil {
				t.Logf("Warning: failed to clean up workspace directory %s: %v", workspaceDir, err)
			}
		} else {
			t.Logf("PRESERVE_TF_RESOURCES is set to true")
			t.Logf("Terraform state preserved in: %s", actualWorkDir)
			t.Logf("To manually destroy resources, run:")
			t.Logf("  cd %s && terraform destroy -var-file=terraform.tfvars.json", actualWorkDir)
		}
	}

	return tf, cleanup
}
