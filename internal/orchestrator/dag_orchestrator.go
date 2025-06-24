package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxkimambo/pd/internal/dag"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/maxkimambo/pd/internal/utils"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// DAGOrchestrator manages the creation and execution of migration DAGs
type DAGOrchestrator struct {
	config    *migrator.Config
	gcpClient *gcp.Clients
}

// NewDAGOrchestrator creates a new DAG orchestrator
func NewDAGOrchestrator(config *migrator.Config, gcpClient *gcp.Clients) *DAGOrchestrator {
	return &DAGOrchestrator{
		config:    config,
		gcpClient: gcpClient,
	}
}

// BuildMigrationDAG constructs a DAG for the migration workflow
func (o *DAGOrchestrator) BuildMigrationDAG(ctx context.Context, instances []*computepb.Instance) (*dag.DAG, error) {
	if logger.User != nil {
		logger.User.Info("Building migration DAG for instance disk migration")
	}
	
	// Create a new DAG
	migrationDAG := dag.NewDAG()
	
	// For each instance, build the migration workflow
	for _, instance := range instances {
		instanceName := instance.GetName()
		zone := utils.ExtractZoneName(instance.GetZone())
		
		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"instance": instanceName,
				"zone":     zone,
			}).Debug("Building workflow for instance")
		}
		
		// Add tasks for this instance
		if err := o.addInstanceWorkflow(ctx, migrationDAG, instance); err != nil {
			return nil, fmt.Errorf("failed to add workflow for instance %s: %w", instanceName, err)
		}
	}
	
	// Validate the DAG
	if err := migrationDAG.Validate(); err != nil {
		return nil, fmt.Errorf("DAG validation failed: %w", err)
	}
	
	if logger.User != nil {
		logger.User.Infof("Successfully built migration DAG with %d nodes", len(migrationDAG.GetAllNodes()))
	}
	return migrationDAG, nil
}

// addInstanceWorkflow adds all tasks for a single instance migration
func (o *DAGOrchestrator) addInstanceWorkflow(ctx context.Context, d *dag.DAG, instance *computepb.Instance) error {
	instanceName := instance.GetName()
	zone := utils.ExtractZoneName(instance.GetZone())
	
	// Get attached disks for this instance
	attachedDisks, err := o.gcpClient.ComputeClient.GetInstanceDisks(ctx, o.config.ProjectID, zone, instanceName)
	if err != nil {
		return fmt.Errorf("failed to get disks for instance %s: %w", instanceName, err)
	}
	
	// Filter disks that need migration (exclude boot disks if not supported and already target type)
	disksToMigrate := o.filterDisksForMigration(attachedDisks)
	
	if len(disksToMigrate) == 0 {
		if logger.User != nil {
			logger.User.Infof("No disks need migration for instance %s", instanceName)
		}
		return nil
	}
	
	// 1. Get instance current state (check if running)
	isRunning := o.gcpClient.ComputeClient.InstanceIsRunning(ctx, instance)
	
	var shutdownID, startupID string
	var diskOperationDeps []string
	
	// 2. If instance is running, add shutdown task
	if isRunning {
		shutdownID = fmt.Sprintf("shutdown_%s", instanceName)
		shutdownTask := dag.NewInstanceStateTask(shutdownID, o.config.ProjectID, zone, instanceName, "stop", o.gcpClient)
		shutdownNode := dag.NewBaseNode(shutdownTask)
		if err := d.AddNode(shutdownNode); err != nil {
			return err
		}
		diskOperationDeps = append(diskOperationDeps, shutdownID)
		
		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"instance": instanceName,
				"task":     shutdownID,
			}).Debug("Added shutdown task")
		}
	}
	
	// Track all disk operations for later dependencies
	var allDiskOperations []string
	
	// For each disk that needs migration
	for _, attachedDisk := range disksToMigrate {
		diskName := extractDiskNameFromSource(attachedDisk.GetSource())
		deviceName := attachedDisk.GetDeviceName()
		
		if diskName == "" {
			continue
		}
		
		diskOperations, err := o.addDiskMigrationWorkflow(d, instanceName, zone, diskName, deviceName, diskOperationDeps)
		if err != nil {
			return fmt.Errorf("failed to add disk migration workflow for %s: %w", diskName, err)
		}
		
		allDiskOperations = append(allDiskOperations, diskOperations...)
	}
	
	// 3. If instance was running, add startup task that depends on all disk operations
	if isRunning {
		startupID = fmt.Sprintf("startup_%s", instanceName)
		startupTask := dag.NewInstanceStateTask(startupID, o.config.ProjectID, zone, instanceName, "start", o.gcpClient)
		startupNode := dag.NewBaseNode(startupTask)
		if err := d.AddNode(startupNode); err != nil {
			return err
		}
		
		// Startup depends on all disk operations completing
		for _, diskOpID := range allDiskOperations {
			if err := d.AddDependency(diskOpID, startupID); err != nil {
				return err
			}
		}
		
		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"instance": instanceName,
				"task":     startupID,
			}).Debug("Added startup task")
		}
	}
	
	return nil
}

// addDiskMigrationWorkflow adds the complete workflow for migrating a single disk
func (o *DAGOrchestrator) addDiskMigrationWorkflow(d *dag.DAG, instanceName, zone, diskName, deviceName string, deps []string) ([]string, error) {
	var operations []string
	
	// 1. Create snapshot task
	snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, time.Now().Unix())
	snapshotID := fmt.Sprintf("snapshot_%s_%s", instanceName, diskName)
	snapshotTask := dag.NewSnapshotTask(snapshotID, o.config.ProjectID, zone, diskName, snapshotName, o.gcpClient, o.config)
	snapshotNode := dag.NewBaseNode(snapshotTask)
	if err := d.AddNode(snapshotNode); err != nil {
		return nil, err
	}
	
	// 2. Detach disk task
	detachID := fmt.Sprintf("detach_%s_%s", instanceName, diskName)
	detachTask := dag.NewDiskAttachmentTask(detachID, o.config.ProjectID, zone, instanceName, diskName, deviceName, "detach", o.gcpClient)
	detachNode := dag.NewBaseNode(detachTask)
	if err := d.AddNode(detachNode); err != nil {
		return nil, err
	}
	
	// Detach depends on snapshot completion and instance shutdown (if applicable)
	if err := d.AddDependency(snapshotID, detachID); err != nil {
		return nil, err
	}
	for _, dep := range deps {
		if err := d.AddDependency(dep, detachID); err != nil {
			return nil, err
		}
	}
	
	// 3. Migrate disk task (delete old, create new from snapshot)
	migrateID := fmt.Sprintf("migrate_%s_%s", instanceName, diskName)
	
	// Get the disk to pass to migration task
	disk, err := o.gcpClient.DiskClient.GetDisk(context.Background(), o.config.ProjectID, zone, diskName)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk %s for migration: %w", diskName, err)
	}
	
	migrateTask := dag.NewDiskMigrationTask(migrateID, o.config.ProjectID, zone, diskName, o.config.TargetDiskType, snapshotName, o.gcpClient, o.config, disk)
	migrateNode := dag.NewBaseNode(migrateTask)
	if err := d.AddNode(migrateNode); err != nil {
		return nil, err
	}
	
	// Migration depends on detach
	if err := d.AddDependency(detachID, migrateID); err != nil {
		return nil, err
	}
	
	// 4. Attach new disk task
	attachID := fmt.Sprintf("attach_%s_%s", instanceName, diskName)
	newDiskName := diskName
	if !o.config.RetainName {
		newDiskName = utils.AddSuffix(diskName, 4)
	}
	
	attachTask := dag.NewDiskAttachmentTask(attachID, o.config.ProjectID, zone, instanceName, newDiskName, deviceName, "attach", o.gcpClient)
	attachNode := dag.NewBaseNode(attachTask)
	if err := d.AddNode(attachNode); err != nil {
		return nil, err
	}
	
	// Attach depends on migration
	if err := d.AddDependency(migrateID, attachID); err != nil {
		return nil, err
	}
	
	// 5. Cleanup snapshot task
	cleanupID := fmt.Sprintf("cleanup_%s_%s", instanceName, diskName)
	cleanupTask := dag.NewCleanupTask(cleanupID, o.config.ProjectID, "snapshot", snapshotName, o.gcpClient)
	cleanupNode := dag.NewBaseNode(cleanupTask)
	if err := d.AddNode(cleanupNode); err != nil {
		return nil, err
	}
	
	// Cleanup depends on successful attach
	if err := d.AddDependency(attachID, cleanupID); err != nil {
		return nil, err
	}
	
	operations = []string{snapshotID, detachID, migrateID, attachID, cleanupID}
	
	if logger.Op != nil {
		logger.Op.WithFields(map[string]interface{}{
			"instance": instanceName,
			"disk":     diskName,
			"workflow": operations,
		}).Debug("Added disk migration workflow")
	}
	
	return operations, nil
}

// ExecuteWithVisualization executes a DAG with optional visualization
func (o *DAGOrchestrator) ExecuteWithVisualization(ctx context.Context, migrationDAG *dag.DAG, visualizeFile string) (*dag.ExecutionResult, error) {
	if logger.User != nil {
		logger.User.Info("Executing migration DAG")
	}
	
	// Apply defaults to config before creating executor
	o.config.ApplyDefaults()
	
	// Create DAG executor
	executor := dag.NewExecutor(migrationDAG, &dag.ExecutorConfig{
		MaxParallelTasks: o.config.MaxParallelTasks,
		TaskTimeout:      o.config.TaskTimeout,
		PollInterval:     100 * time.Millisecond,
	})

	// Enable visualization if requested
	if visualizeFile != "" {
		o.EnableVisualization(executor, visualizeFile)
		if logger.User != nil {
			logger.User.Infof("DAG visualization enabled: updates will be written to %s every 5 seconds", visualizeFile)
		}
	}

	// Execute the DAG
	result, err := executor.Execute(ctx)
	if err != nil {
		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"error": err.Error(),
			}).Error("DAG execution failed")
		}
		return result, fmt.Errorf("DAG execution failed: %w", err)
	}

	if logger.User != nil {
		logger.User.Infof("DAG execution completed. Success: %t, Duration: %v", result.Success, result.ExecutionTime)
	}
	
	// Log any failed nodes
	if logger.Op != nil {
		for nodeID, nodeResult := range result.NodeResults {
			if !nodeResult.Success {
				logger.Op.WithFields(map[string]interface{}{
					"node":  nodeID,
					"error": nodeResult.Error,
				}).Error("Node execution failed")
			}
		}
	}

	// Final visualization export if requested
	if visualizeFile != "" && result != nil {
		if logger.User != nil {
			logger.User.Infof("Exporting final DAG visualization to: %s", visualizeFile)
		}
		
		// Export final state using the executor's visualization
		visualization := executor.GetVisualization()
		if strings.HasSuffix(visualizeFile, ".json") {
			if err := visualization.ExportToJSON(visualizeFile); err != nil {
				if logger.User != nil {
					logger.User.Warnf("Failed to export final JSON visualization: %v", err)
				}
			}
		} else if strings.HasSuffix(visualizeFile, ".dot") {
			if err := visualization.ExportToDOT(visualizeFile); err != nil {
				if logger.User != nil {
					logger.User.Warnf("Failed to export final DOT visualization: %v", err)
				}
			}
		} else if strings.HasSuffix(visualizeFile, ".txt") {
			if err := visualization.ExportToText(visualizeFile); err != nil {
				if logger.User != nil {
					logger.User.Warnf("Failed to export final text visualization: %v", err)
				}
			}
		} else {
			// Default to JSON if no extension or unknown extension
			jsonFile := visualizeFile
			if !strings.Contains(visualizeFile, ".") {
				jsonFile += ".json"
			}
			if err := visualization.ExportToJSON(jsonFile); err != nil {
				if logger.User != nil {
					logger.User.Warnf("Failed to export final JSON visualization: %v", err)
				}
			}
		}
	}

	return result, nil
}

// EnableVisualization sets up DAG execution visualization
func (o *DAGOrchestrator) EnableVisualization(executor *dag.Executor, visualizeFile string) {
	if visualizeFile == "" {
		return
	}

	if logger.User != nil {
		logger.User.Infof("Enabling DAG visualization: %s", visualizeFile)
	}

	// Enable visualization with 5-second update intervals
	executor.EnableVisualization(visualizeFile, 5*time.Second)
}

// ExecuteMigrationDAG runs the migration workflow (backward compatibility wrapper)
func (o *DAGOrchestrator) ExecuteMigrationDAG(ctx context.Context, migrationDAG *dag.DAG) (*dag.ExecutionResult, error) {
	// Use the new visualization-enabled execution without visualization file
	// This maintains backward compatibility while leveraging the new infrastructure
	return o.ExecuteWithVisualization(ctx, migrationDAG, "")
}

// filterDisksForMigration returns only disks that need to be migrated
func (o *DAGOrchestrator) filterDisksForMigration(attachedDisks []*computepb.AttachedDisk) []*computepb.AttachedDisk {
	var disksToMigrate []*computepb.AttachedDisk
	
	for _, attachedDisk := range attachedDisks {
		// Skip if it's a boot disk and we don't want to migrate boot disks
		if attachedDisk.GetBoot() {
			// Log if logger is available (not in unit tests)
			if logger.Op != nil {
				logger.Op.WithFields(map[string]interface{}{
					"disk": extractDiskNameFromSource(attachedDisk.GetSource()),
				}).Debug("Skipping boot disk")
			}
			continue
		}
		
		// For now, include all non-boot disks
		// TODO: Add logic to check if disk is already target type
		disksToMigrate = append(disksToMigrate, attachedDisk)
	}
	
	return disksToMigrate
}

// extractDiskNameFromSource extracts the disk name from a source URL
// URL format: projects/PROJECT/zones/ZONE/disks/DISK_NAME
func extractDiskNameFromSource(source string) string {
	if source == "" {
		return ""
	}
	
	parts := strings.Split(source, "/")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	
	return source
}