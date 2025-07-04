package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Terraform struct {
	workDir string
}

func New(workDir string) *Terraform {
	return &Terraform{
		workDir: workDir,
	}
}

func (t *Terraform) Init(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "terraform", "init")
	cmd.Dir = t.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (t *Terraform) Apply(ctx context.Context, vars map[string]any) (map[string]any, error) {
	varFile, err := t.writeVarsFile(vars)
	if err != nil {
		return nil, fmt.Errorf("failed to write vars file: %w", err)
	}
	defer os.Remove(varFile)

	cmd := exec.CommandContext(ctx, "terraform", "apply", "-auto-approve", "-var-file="+varFile)
	cmd.Dir = t.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("terraform apply failed: %w", err)
	}

	return t.Output(ctx)
}

func (t *Terraform) Destroy(ctx context.Context, vars map[string]any) error {
	varFile, err := t.writeVarsFile(vars)
	if err != nil {
		return fmt.Errorf("failed to write vars file: %w", err)
	}
	defer os.Remove(varFile)

	cmd := exec.CommandContext(ctx, "terraform", "destroy", "-auto-approve", "-var-file="+varFile)
	cmd.Dir = t.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform destroy failed: %w", err)
	}

	return nil
}

func (t *Terraform) Output(ctx context.Context) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = t.workDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform output failed: %w", err)
	}

	var outputs map[string]any
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, fmt.Errorf("failed to parse terraform output: %w", err)
	}

	result := make(map[string]any)
	for key, value := range outputs {
		if m, ok := value.(map[string]any); ok {
			if v, exists := m["value"]; exists {
				result[key] = v
			}
		}
	}

	return result, nil
}

func (t *Terraform) writeVarsFile(vars map[string]any) (string, error) {
	data, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return "", err
	}

	varFile := filepath.Join(t.workDir, "terraform.tfvars.json")
	if err := os.WriteFile(varFile, data, 0644); err != nil {
		return "", err
	}

	return varFile, nil
}

func CreateTestWorkspace(scenarioPath, targetDir string) error {
	// Get absolute path of the scenario
	absScenarioPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if the scenario directory exists
	if _, err := os.Stat(absScenarioPath); os.IsNotExist(err) {
		return fmt.Errorf("scenario path does not exist: %s", absScenarioPath)
	}

	// Copy the entire terraform directory structure to the target directory
	// This ensures module references work correctly
	terraformRoot := filepath.Dir(filepath.Dir(absScenarioPath))
	
	// Debug: Check if terraformRoot exists
	if _, err := os.Stat(terraformRoot); os.IsNotExist(err) {
		return fmt.Errorf("terraform root does not exist: %s", terraformRoot)
	}
	
	if err := copyDir(terraformRoot, targetDir); err != nil {
		return fmt.Errorf("failed to copy terraform files from %s to %s: %w", terraformRoot, targetDir, err)
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .terraform directories but not .terraform.lock.hcl files
		if strings.Contains(path, ".terraform") && !strings.HasSuffix(path, ".terraform.lock.hcl") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Only copy .tf, .hcl, and .json files
		if strings.HasSuffix(path, ".tf") || strings.HasSuffix(path, ".hcl") || 
		   strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".tfvars") {
			return copyFile(path, dstPath)
		}

		return nil
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}