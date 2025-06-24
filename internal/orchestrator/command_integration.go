package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxkimambo/pd/internal/dag"
	"github.com/maxkimambo/pd/internal/logger"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// ExecuteInstanceMigrations orchestrates compute instance migrations using DAG
// This is the high-level entry point for CLI commands
func (o *DAGOrchestrator) ExecuteInstanceMigrations(ctx context.Context, instances []*computepb.Instance) (*dag.ExecutionResult, error) {
	return o.ExecuteInstanceMigrationsWithVisualization(ctx, instances, "")
}

// ExecuteInstanceMigrationsWithVisualization orchestrates compute instance migrations using DAG with optional visualization
func (o *DAGOrchestrator) ExecuteInstanceMigrationsWithVisualization(ctx context.Context, instances []*computepb.Instance, visualizeFile string) (*dag.ExecutionResult, error) {
	if logger.User != nil {
		logger.User.Infof("Starting DAG-based migration for %d instance(s)", len(instances))
	}

	// Build DAG from instances
	migrationDAG, err := o.BuildMigrationDAG(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("failed to build migration DAG: %w", err)
	}

	if logger.User != nil {
		allNodes := migrationDAG.GetAllNodes()
		logger.User.Infof("Built migration DAG with %d tasks", len(allNodes))
	}

	// Execute DAG with visualization
	result, err := o.ExecuteWithVisualization(ctx, migrationDAG, visualizeFile)
	if err != nil {
		return nil, fmt.Errorf("DAG execution failed: %w", err)
	}

	// Log execution summary
	if logger.User != nil {
		completed := 0
		failed := 0
		for _, nodeResult := range result.NodeResults {
			if nodeResult.Success {
				completed++
			} else {
				failed++
			}
		}
		
		if result.Success {
			logger.User.Successf("DAG execution completed successfully: %d tasks completed", completed)
		} else {
			logger.User.Errorf("DAG execution completed with errors: %d succeeded, %d failed", completed, failed)
		}
	}

	return result, nil
}

// ExecuteDiskMigrations orchestrates standalone disk migrations using DAG
// This handles detached disk migrations with DAG-based orchestration
func (o *DAGOrchestrator) ExecuteDiskMigrations(ctx context.Context, disks []*computepb.Disk) (*dag.ExecutionResult, error) {
	return o.ExecuteDiskMigrationsWithVisualization(ctx, disks, "")
}

// ExecuteDiskMigrationsWithVisualization orchestrates standalone disk migrations using DAG with optional visualization
func (o *DAGOrchestrator) ExecuteDiskMigrationsWithVisualization(ctx context.Context, disks []*computepb.Disk, visualizeFile string) (*dag.ExecutionResult, error) {
	// Ensure config and gcpClient are available
	if o.config == nil || o.gcpClient == nil {
		return nil, fmt.Errorf("orchestrator not properly initialized")
	}
	if logger.User != nil {
		logger.User.Infof("Starting DAG-based migration for %d disk(s)", len(disks))
	}

	// Build DAG from disks
	migrationDAG, err := o.BuildDiskMigrationDAG(ctx, disks)
	if err != nil {
		return nil, fmt.Errorf("failed to build disk migration DAG: %w", err)
	}

	if logger.User != nil {
		allNodes := migrationDAG.GetAllNodes()
		logger.User.Infof("Built disk migration DAG with %d tasks", len(allNodes))
	}

	// Execute DAG with visualization
	result, err := o.ExecuteWithVisualization(ctx, migrationDAG, visualizeFile)
	if err != nil {
		return nil, fmt.Errorf("DAG execution failed: %w", err)
	}

	// Log execution summary
	if logger.User != nil {
		completed := 0
		failed := 0
		for _, nodeResult := range result.NodeResults {
			if nodeResult.Success {
				completed++
			} else {
				failed++
			}
		}
		
		if result.Success {
			logger.User.Successf("Disk migration completed successfully: %d tasks completed", completed)
		} else {
			logger.User.Errorf("Disk migration completed with errors: %d succeeded, %d failed", completed, failed)
		}
	}

	return result, nil
}

// BuildDiskMigrationDAG creates a DAG for standalone disk migrations
func (o *DAGOrchestrator) BuildDiskMigrationDAG(ctx context.Context, disks []*computepb.Disk) (*dag.DAG, error) {
	if logger.User != nil {
		logger.User.Info("Building migration DAG for standalone disk migration")
	}

	// Create a new DAG
	migrationDAG := dag.NewDAG()

	// For each disk, create migration tasks
	for _, disk := range disks {
		diskName := disk.GetName()
		zone := extractZoneFromSelfLink(disk.GetZone())

		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"disk": diskName,
				"zone": zone,
			}).Debug("Building workflow for disk")
		}

		// Create snapshot task
		snapshotTaskID := fmt.Sprintf("snapshot-%s", diskName)
		snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, time.Now().Unix())
		snapshotTask := dag.NewSnapshotTask(snapshotTaskID, o.config.ProjectID, zone, diskName, snapshotName, o.gcpClient, o.config)
		snapshotNode := dag.NewBaseNode(snapshotTask)

		err := migrationDAG.AddNode(snapshotNode)
		if err != nil {
			return nil, fmt.Errorf("failed to add snapshot task for disk %s: %w", diskName, err)
		}

		// Create disk migration task
		migrationTaskID := fmt.Sprintf("migrate-%s", diskName)
		migrationTask := dag.NewDiskMigrationTask(migrationTaskID, o.config.ProjectID, zone, diskName, o.config.TargetDiskType, snapshotName, o.gcpClient, o.config, disk)
		migrationNode := dag.NewBaseNode(migrationTask)

		err = migrationDAG.AddNode(migrationNode)
		if err != nil {
			return nil, fmt.Errorf("failed to add migration task for disk %s: %w", diskName, err)
		}

		// Add dependency: snapshot -> migration
		err = migrationDAG.AddDependency(snapshotTaskID, migrationTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to add dependency for disk %s: %w", diskName, err)
		}

		// Create cleanup task
		cleanupTaskID := fmt.Sprintf("cleanup-%s", diskName)
		cleanupTask := dag.NewCleanupTask(cleanupTaskID, o.config.ProjectID, "snapshot", snapshotName, o.gcpClient)
		cleanupNode := dag.NewBaseNode(cleanupTask)

		err = migrationDAG.AddNode(cleanupNode)
		if err != nil {
			return nil, fmt.Errorf("failed to add cleanup task for disk %s: %w", diskName, err)
		}

		// Add dependency: migration -> cleanup
		err = migrationDAG.AddDependency(migrationTaskID, cleanupTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to add cleanup dependency for disk %s: %w", diskName, err)
		}

		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"disk":     diskName,
				"tasks":    3,
				"sequence": fmt.Sprintf("%s -> %s -> %s", snapshotTaskID, migrationTaskID, cleanupTaskID),
			}).Debug("Added disk migration workflow")
		}
	}

	if logger.User != nil {
		allNodes := migrationDAG.GetAllNodes()
		logger.User.Infof("Built migration DAG with %d total tasks for %d disks", len(allNodes), len(disks))
	}

	return migrationDAG, nil
}


// ProcessExecutionResults handles the results from DAG execution
func (o *DAGOrchestrator) ProcessExecutionResults(result *dag.ExecutionResult) error {
	if result == nil {
		return fmt.Errorf("execution result is nil")
	}

	// Report detailed results
	if logger.User != nil {
		logger.User.Infof("Migration execution completed in %v", result.ExecutionTime)
		
		if result.Success {
			logger.User.Success("All migration tasks completed successfully")
		} else {
			logger.User.Error("Migration completed with errors")
			
			// Report failed tasks
			for nodeID, nodeResult := range result.NodeResults {
				if nodeResult.Error != nil {
					logger.User.Errorf("Task %s failed: %v", nodeID, nodeResult.Error)
				}
			}
		}
	}

	// Return error if execution failed
	if !result.Success {
		return fmt.Errorf("migration execution failed: %v", result.Error)
	}

	return nil
}

// extractZoneFromSelfLink extracts zone name from a GCP self link
func extractZoneFromSelfLink(selfLink string) string {
	if selfLink == "" {
		return ""
	}
	
	parts := strings.Split(selfLink, "/")
	for i, part := range parts {
		if part == "zones" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	
	return ""
}