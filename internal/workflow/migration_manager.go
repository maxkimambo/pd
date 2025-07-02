package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/taskmanager"
)

// MigrationManager coordinates workflow creation and execution for disk migrations
type MigrationManager struct {
	gcpClient *gcp.Clients
	config    *migrator.Config
	factory   *WorkflowFactory
}

// NewMigrationManager creates a new MigrationManager instance
func NewMigrationManager(gcpClient *gcp.Clients, config *migrator.Config) *MigrationManager {
	return &MigrationManager{
		gcpClient: gcpClient,
		config:    config,
		factory:   NewWorkflowFactory(gcpClient, config),
	}
}

// WorkflowResult represents the outcome of a workflow execution
type WorkflowResult struct {
	WorkflowID    string
	StartTime     time.Time
	EndTime       time.Time
	SharedContext *taskmanager.SharedContext
	Error         error
	Success       bool
}

// CreateInstanceMigrationWorkflow creates a workflow for migrating disks attached to an instance
func (m *MigrationManager) CreateInstanceMigrationWorkflow(instance *computepb.Instance) (*taskmanager.Workflow, error) {
	if instance == nil || instance.Name == nil {
		return nil, fmt.Errorf("instance cannot be nil and must have a name")
	}

	workflowID := fmt.Sprintf("migrate-instance-%s-%d", *instance.Name, time.Now().Unix())
	logger.Debugf("Creating workflow %s for instance %s", workflowID, *instance.Name)

	return m.factory.CreateComputeDiskMigrationWorkflow(workflowID, instance)
}

// ExecuteWorkflow runs a workflow and returns the result
func (m *MigrationManager) ExecuteWorkflow(ctx context.Context, workflow *taskmanager.Workflow) (*WorkflowResult, error) {
	startTime := time.Now()

	// Create shared context with necessary data
	sharedCtx := taskmanager.NewSharedContext()
	sharedCtx.Set("gcp_client", m.gcpClient)

	// Convert config to map for easier access in tasks
	configMap := map[string]interface{}{
		"ProjectID":      m.config.ProjectID,
		"TargetDiskType": m.config.TargetDiskType,
		"LabelFilter":    m.config.LabelFilter,
		"KmsKey":         m.config.KmsKey,
		"KmsKeyRing":     m.config.KmsKeyRing,
		"KmsLocation":    m.config.KmsLocation,
		"KmsProject":     m.config.KmsProject,
		"Region":         m.config.Region,
		"Zone":           m.config.Zone,
		"AutoApproveAll": m.config.AutoApproveAll,
		"Concurrency":    m.config.Concurrency,
		"RetainName":     m.config.RetainName,
		"Instances":      m.config.Instances,
		"Throughput":     m.config.Throughput,
		"Iops":           m.config.Iops,
	}
	sharedCtx.Set("config", configMap)
	sharedCtx.Set("workflow_start_time", startTime)

	logger.Starting(fmt.Sprintf("Starting workflow: %s", workflow.ID))

	// Execute the workflow
	err := workflow.Execute(ctx, sharedCtx)

	endTime := time.Now()

	// Create result
	result := &WorkflowResult{
		WorkflowID:    workflow.ID,
		StartTime:     startTime,
		EndTime:       endTime,
		SharedContext: sharedCtx,
		Error:         err,
		Success:       err == nil,
	}

	if err != nil {
		logger.Errorf("Workflow %s failed: %v", workflow.ID, err)
	} else {
		logger.Successf("Workflow %s completed successfully in %v", workflow.ID, endTime.Sub(startTime))
	}

	return result, err
}

// ReportResults provides a summary of the workflow execution
func (m *MigrationManager) ReportResults(result *WorkflowResult) {
	logger.Info("\n📋 Workflow Execution Report\n" + strings.Repeat("=", 35))
	logger.Infof("Workflow ID: %s", result.WorkflowID)
	logger.Infof("Duration: %v", result.EndTime.Sub(result.StartTime))
	logger.Infof("Status: %s", m.getStatusString(result.Success))

	// Extract and report migration results if available
	if migrationResults, ok := result.SharedContext.Get("migration_results"); ok {
		if results, ok := migrationResults.([]migrator.MigrationResult); ok {
			logger.Info("\nMigration Results:")
			for _, res := range results {
				if res.ErrorMessage != "" {
					logger.Errorf("  - %s: FAILED (%s)", res.DiskName, res.ErrorMessage)
				} else {
					logger.Successf("  - %s: SUCCESS (migrated to %s)", res.DiskName, res.NewDiskName)
				}
			}
		}
	}

	// Report any verification errors
	if verificationErrors, ok := result.SharedContext.Get("verification_errors"); ok {
		if errors, ok := verificationErrors.([]error); ok && len(errors) > 0 {
			logger.Warn("\nVerification Issues:")
			for _, err := range errors {
				logger.Warnf("  - %v", err)
			}
		}
	}

	// Report cleanup status
	if cleanupStatus, ok := result.SharedContext.Get("cleanup_status"); ok {
		logger.Infof("\nCleanup Status: %v", cleanupStatus)
	}

	if result.Error != nil {
		logger.Errorf("\nWorkflow Error: %v", result.Error)
	}

	logger.Info(strings.Repeat("=", 35))
}

// getStatusString returns a human-readable status string
func (m *MigrationManager) getStatusString(success bool) string {
	if success {
		return "COMPLETED"
	}
	return "FAILED"
}

// GetWorkflowStatus returns the current status of a workflow (for future use)
func (m *MigrationManager) GetWorkflowStatus(workflowID string) (string, error) {
	// This could be extended to support persistent workflow state
	return "UNKNOWN", fmt.Errorf("workflow status tracking not yet implemented")
}
