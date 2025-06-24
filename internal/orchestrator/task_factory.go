package orchestrator

import (
	"fmt"

	"github.com/maxkimambo/pd/internal/dag"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// TaskFactory creates task instances for the migration workflow
type TaskFactory struct {
	config    *migrator.Config
	gcpClient *gcp.Clients
}

// NewTaskFactory creates a new task factory
func NewTaskFactory(config *migrator.Config, gcpClient *gcp.Clients) *TaskFactory {
	return &TaskFactory{
		config:    config,
		gcpClient: gcpClient,
	}
}

// CreateDiscoveryTask creates a task for resource discovery
func (f *TaskFactory) CreateDiscoveryTask(resourceType string) dag.Node {
	id := fmt.Sprintf("discover_%s_%s", resourceType, f.config.Location())
	task := dag.NewDiscoveryTask(id, f.config, f.gcpClient, resourceType)
	return dag.NewBaseNode(task)
}

// CreateSnapshotTask creates a task for snapshot creation
func (f *TaskFactory) CreateSnapshotTask(diskName, snapshotName string) dag.Node {
	id := fmt.Sprintf("snapshot_%s", diskName)
	task := dag.NewSnapshotTask(id, f.config.ProjectID, f.config.Zone, diskName, snapshotName, f.gcpClient, f.config)
	return dag.NewBaseNode(task)
}

// CreateInstanceStateTask creates a task for instance state operations
func (f *TaskFactory) CreateInstanceStateTask(instanceName, action string) dag.Node {
	id := fmt.Sprintf("%s_%s", action, instanceName)
	task := dag.NewInstanceStateTask(id, f.config.ProjectID, f.config.Zone, instanceName, action, f.gcpClient)
	return dag.NewBaseNode(task)
}

// CreateDiskAttachmentTask creates a task for disk attach/detach operations
func (f *TaskFactory) CreateDiskAttachmentTask(instanceName, diskName, deviceName, action string) dag.Node {
	id := fmt.Sprintf("%s_%s_%s", action, instanceName, diskName)
	task := dag.NewDiskAttachmentTask(id, f.config.ProjectID, f.config.Zone, instanceName, diskName, deviceName, action, f.gcpClient)
	return dag.NewBaseNode(task)
}

// CreateDiskMigrationTask creates a task for disk migration
func (f *TaskFactory) CreateDiskMigrationTask(diskName, snapshotName string, disk *computepb.Disk) dag.Node {
	id := fmt.Sprintf("migrate_%s", diskName)
	task := dag.NewDiskMigrationTask(id, f.config.ProjectID, f.config.Zone, diskName, f.config.TargetDiskType, snapshotName, f.gcpClient, f.config, disk)
	return dag.NewBaseNode(task)
}

// CreateCleanupTask creates a task for resource cleanup
func (f *TaskFactory) CreateCleanupTask(resourceType, resourceID string) dag.Node {
	id := fmt.Sprintf("cleanup_%s_%s", resourceType, resourceID)
	task := dag.NewCleanupTask(id, f.config.ProjectID, resourceType, resourceID, f.gcpClient)
	return dag.NewBaseNode(task)
}

// CreateInstanceWorkflow creates a complete workflow for an instance migration
func (f *TaskFactory) CreateInstanceWorkflow(instance *computepb.Instance) ([]dag.Node, map[string][]string, error) {
	instanceName := instance.GetName()
	var nodes []dag.Node
	dependencies := make(map[string][]string)
	
	// Check if instance is running (simplified for factory)
	isRunning := instance.GetStatus() == "RUNNING"
	
	var shutdownNodeID, startupNodeID string
	
	// 1. Create shutdown task if instance is running
	if isRunning {
		shutdownNode := f.CreateInstanceStateTask(instanceName, "stop")
		shutdownNodeID = shutdownNode.ID()
		nodes = append(nodes, shutdownNode)
	}
	
	// 2. Get attached disks and create migration workflow for each
	if instance.Disks != nil {
		for _, attachedDisk := range instance.Disks {
			// Skip boot disks
			if attachedDisk.GetBoot() {
				continue
			}
			
			diskName := extractDiskNameFromSource(attachedDisk.GetSource())
			deviceName := attachedDisk.GetDeviceName()
			
			if diskName == "" {
				continue
			}
			
			// Create disk migration workflow
			diskNodes, diskDeps, err := f.createDiskMigrationWorkflow(instanceName, diskName, deviceName, shutdownNodeID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create disk migration workflow for %s: %w", diskName, err)
			}
			
			nodes = append(nodes, diskNodes...)
			
			// Merge dependencies
			for nodeID, deps := range diskDeps {
				dependencies[nodeID] = append(dependencies[nodeID], deps...)
			}
		}
	}
	
	// 3. Create startup task if instance was running
	if isRunning {
		startupNode := f.CreateInstanceStateTask(instanceName, "start")
		startupNodeID = startupNode.ID()
		nodes = append(nodes, startupNode)
		
		// Startup depends on all disk operations completing
		for _, node := range nodes {
			if dag.GetTaskType(node.GetTask()) == "DiskAttachment" && 
			   dag.GetTaskDescription(node.GetTask()) != "" && 
			   fmt.Sprintf("Attach disk") == dag.GetTaskDescription(node.GetTask())[:10] {
				dependencies[startupNodeID] = append(dependencies[startupNodeID], node.ID())
			}
		}
	}
	
	return nodes, dependencies, nil
}

// createDiskMigrationWorkflow creates the complete workflow for migrating a single disk
func (f *TaskFactory) createDiskMigrationWorkflow(instanceName, diskName, deviceName, shutdownNodeID string) ([]dag.Node, map[string][]string, error) {
	var nodes []dag.Node
	dependencies := make(map[string][]string)
	
	// 1. Create snapshot
	snapshotName := fmt.Sprintf("pd-migrate-%s-%d", diskName, 1234567890) // Use timestamp in real implementation
	snapshotNode := f.CreateSnapshotTask(diskName, snapshotName)
	snapshotNodeID := snapshotNode.ID()
	nodes = append(nodes, snapshotNode)
	
	// 2. Detach disk
	detachNode := f.CreateDiskAttachmentTask(instanceName, diskName, deviceName, "detach")
	detachNodeID := detachNode.ID()
	nodes = append(nodes, detachNode)
	
	// Detach depends on snapshot completion
	dependencies[detachNodeID] = append(dependencies[detachNodeID], snapshotNodeID)
	
	// If there's a shutdown task, detach also depends on it
	if shutdownNodeID != "" {
		dependencies[detachNodeID] = append(dependencies[detachNodeID], shutdownNodeID)
	}
	
	// 3. Migrate disk (requires getting the disk first)
	// For factory pattern, we'll create a wrapper that gets the disk during execution
	migrateNode := f.createDiskMigrationWrapper(diskName, snapshotName)
	migrateNodeID := migrateNode.ID()
	nodes = append(nodes, migrateNode)
	
	// Migration depends on detach
	dependencies[migrateNodeID] = append(dependencies[migrateNodeID], detachNodeID)
	
	// 4. Attach new disk
	newDiskName := diskName
	if !f.config.RetainName {
		newDiskName = diskName + "-new" // Simplified for factory
	}
	
	attachNode := f.CreateDiskAttachmentTask(instanceName, newDiskName, deviceName, "attach")
	attachNodeID := attachNode.ID()
	nodes = append(nodes, attachNode)
	
	// Attach depends on migration
	dependencies[attachNodeID] = append(dependencies[attachNodeID], migrateNodeID)
	
	// 5. Cleanup snapshot
	cleanupNode := f.CreateCleanupTask("snapshot", snapshotName)
	cleanupNodeID := cleanupNode.ID()
	nodes = append(nodes, cleanupNode)
	
	// Cleanup depends on successful attach
	dependencies[cleanupNodeID] = append(dependencies[cleanupNodeID], attachNodeID)
	
	return nodes, dependencies, nil
}

// createDiskMigrationWrapper creates a wrapper task that handles disk migration
// This is a simplified version for the factory pattern
func (f *TaskFactory) createDiskMigrationWrapper(diskName, snapshotName string) dag.Node {
	id := fmt.Sprintf("migrate_%s", diskName)
	task := &DiskMigrationWrapper{
		id:           id,
		name:         fmt.Sprintf("Migrate disk %s to %s", diskName, f.config.TargetDiskType),
		factory:      f,
		diskName:     diskName,
		snapshotName: snapshotName,
	}
	return dag.NewBaseNode(task)
}

// CreateBatchSnapshotTask creates a task for creating multiple snapshots in parallel
func (f *TaskFactory) CreateBatchSnapshotTask(diskNames []string) dag.Node {
	id := fmt.Sprintf("batch_snapshot_%d_disks", len(diskNames))
	task := &BatchSnapshotTask{
		id:        id,
		name:      fmt.Sprintf("Create snapshots for %d disks", len(diskNames)),
		factory:   f,
		diskNames: diskNames,
	}
	return dag.NewBaseNode(task)
}

// GetConfig returns the factory's configuration
func (f *TaskFactory) GetConfig() *migrator.Config {
	return f.config
}

// GetGCPClient returns the factory's GCP client
func (f *TaskFactory) GetGCPClient() *gcp.Clients {
	return f.gcpClient
}