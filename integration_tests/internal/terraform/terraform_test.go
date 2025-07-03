package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateTestWorkspace(t *testing.T) {
	// Create a test scenario structure
	tempBase, err := os.MkdirTemp("", "terraform-test-base-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBase)

	// Create test structure
	scenarios := filepath.Join(tempBase, "scenarios")
	modules := filepath.Join(tempBase, "modules")
	testScenario := filepath.Join(scenarios, "test-scenario")
	testModule := filepath.Join(modules, "test-module")

	for _, dir := range []string{testScenario, testModule} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create test files
	mainTf := filepath.Join(testScenario, "main.tf")
	if err := os.WriteFile(mainTf, []byte(`module "test" { source = "../../modules/test-module" }`), 0644); err != nil {
		t.Fatal(err)
	}

	moduleTf := filepath.Join(testModule, "main.tf")
	if err := os.WriteFile(moduleTf, []byte(`resource "null_resource" "test" {}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Test CreateTestWorkspace
	workDir, err := CreateTestWorkspace(testScenario)
	if err != nil {
		t.Fatalf("CreateTestWorkspace failed: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(filepath.Dir(workDir)))

	// Verify structure
	if _, err := os.Stat(workDir); err != nil {
		t.Errorf("Scenario directory not found: %v", err)
	}

	mainPath := filepath.Join(workDir, "main.tf")
	if _, err := os.Stat(mainPath); err != nil {
		t.Errorf("main.tf not found in scenario: %v", err)
	}

	moduleDir := filepath.Join(filepath.Dir(filepath.Dir(workDir)), "modules", "test-module")
	if _, err := os.Stat(moduleDir); err != nil {
		t.Errorf("Module directory not found: %v", err)
	}
}