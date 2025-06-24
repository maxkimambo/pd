package orchestrator

import (
	"testing"

	"github.com/maxkimambo/pd/internal/dag"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/stretchr/testify/assert"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/protobuf/proto"
)

func TestNewTaskFactory(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	
	factory := NewTaskFactory(config, gcpClient)
	
	assert.NotNil(t, factory)
	assert.Equal(t, config, factory.config)
	assert.Equal(t, gcpClient, factory.gcpClient)
}

func TestCreateDiscoveryTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	node := factory.CreateDiscoveryTask("disks")
	
	assert.NotNil(t, node)
	assert.Equal(t, "discover_disks_us-central1-a", node.ID())
	assert.Equal(t, "Discovery", dag.GetTaskType(node.GetTask()))
	assert.Contains(t, dag.GetTaskDescription(node.GetTask()), "Discover disks")
}

func TestCreateSnapshotTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	node := factory.CreateSnapshotTask("test-disk", "test-snapshot")
	
	assert.NotNil(t, node)
	assert.Equal(t, "snapshot_test-disk", node.ID())
	assert.Equal(t, "Snapshot", dag.GetTaskType(node.GetTask()))
	assert.Contains(t, dag.GetTaskDescription(node.GetTask()), "Create snapshot test-snapshot of disk test-disk")
}

func TestCreateInstanceStateTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	tests := []struct {
		name           string
		instanceName   string
		action         string
		expectedID     string
		expectedDesc   string
	}{
		{
			name:         "stop instance",
			instanceName: "test-instance",
			action:       "stop",
			expectedID:   "stop_test-instance",
			expectedDesc: "Stop instance test-instance",
		},
		{
			name:         "start instance",
			instanceName: "test-instance",
			action:       "start",
			expectedID:   "start_test-instance",
			expectedDesc: "Start instance test-instance",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := factory.CreateInstanceStateTask(tt.instanceName, tt.action)
			
			assert.NotNil(t, node)
			assert.Equal(t, tt.expectedID, node.ID())
			assert.Equal(t, "InstanceState", dag.GetTaskType(node.GetTask()))
			assert.Equal(t, tt.expectedDesc, dag.GetTaskDescription(node.GetTask()))
		})
	}
}

func TestCreateDiskAttachmentTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	tests := []struct {
		name         string
		instanceName string
		diskName     string
		deviceName   string
		action       string
		expectedID   string
		expectedDesc string
	}{
		{
			name:         "attach disk",
			instanceName: "test-instance",
			diskName:     "test-disk",
			deviceName:   "sdb",
			action:       "attach",
			expectedID:   "attach_test-instance_test-disk",
			expectedDesc: "Attach disk test-disk to instance test-instance",
		},
		{
			name:         "detach disk",
			instanceName: "test-instance",
			diskName:     "test-disk",
			deviceName:   "sdb",
			action:       "detach",
			expectedID:   "detach_test-instance_test-disk",
			expectedDesc: "Detach disk test-disk to instance test-instance",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := factory.CreateDiskAttachmentTask(tt.instanceName, tt.diskName, tt.deviceName, tt.action)
			
			assert.NotNil(t, node)
			assert.Equal(t, tt.expectedID, node.ID())
			assert.Equal(t, "DiskAttachment", dag.GetTaskType(node.GetTask()))
			assert.Equal(t, tt.expectedDesc, dag.GetTaskDescription(node.GetTask()))
		})
	}
}

func TestCreateDiskMigrationTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	disk := &computepb.Disk{
		Name:   proto.String("test-disk"),
		SizeGb: proto.Int64(100),
	}
	
	node := factory.CreateDiskMigrationTask("test-disk", "test-snapshot", disk)
	
	assert.NotNil(t, node)
	assert.Equal(t, "migrate_test-disk", node.ID())
	assert.Equal(t, "DiskMigration", dag.GetTaskType(node.GetTask()))
	assert.Contains(t, dag.GetTaskDescription(node.GetTask()), "Migrate disk test-disk to pd-ssd")
}

func TestCreateCleanupTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	node := factory.CreateCleanupTask("snapshot", "test-snapshot")
	
	assert.NotNil(t, node)
	assert.Equal(t, "cleanup_snapshot_test-snapshot", node.ID())
	assert.Equal(t, "Cleanup", dag.GetTaskType(node.GetTask()))
	assert.Equal(t, "Clean up snapshot test-snapshot", dag.GetTaskDescription(node.GetTask()))
}

func TestCreateBatchSnapshotTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	diskNames := []string{"disk1", "disk2", "disk3"}
	node := factory.CreateBatchSnapshotTask(diskNames)
	
	assert.NotNil(t, node)
	assert.Equal(t, "batch_snapshot_3_disks", node.ID())
	assert.Equal(t, "BatchSnapshot", dag.GetTaskType(node.GetTask()))
	assert.Equal(t, "Create snapshots for 3 disks", dag.GetTaskDescription(node.GetTask()))
}

func TestCreateValidationTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	tests := []struct {
		name         string
		resourceType string
		resourceID   string
		expectedID   string
		expectedDesc string
	}{
		{
			name:         "validate disk",
			resourceType: "disk",
			resourceID:   "test-disk",
			expectedID:   "validate_disk_test-disk",
			expectedDesc: "Validate disk test-disk",
		},
		{
			name:         "validate instance",
			resourceType: "instance",
			resourceID:   "test-instance",
			expectedID:   "validate_instance_test-instance",
			expectedDesc: "Validate instance test-instance",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := factory.CreateValidationTask(tt.resourceType, tt.resourceID)
			
			assert.NotNil(t, node)
			assert.Equal(t, tt.expectedID, node.ID())
			assert.Equal(t, "Validation", dag.GetTaskType(node.GetTask()))
			assert.Equal(t, tt.expectedDesc, dag.GetTaskDescription(node.GetTask()))
		})
	}
}

func TestCreatePreflightCheckTask(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	instances := []string{"instance1", "instance2"}
	node := factory.CreatePreflightCheckTask(instances)
	
	assert.NotNil(t, node)
	assert.Equal(t, "preflight_check_2_instances", node.ID())
	assert.Equal(t, "PreflightCheck", dag.GetTaskType(node.GetTask()))
	assert.Equal(t, "Preflight check for 2 instances", dag.GetTaskDescription(node.GetTask()))
}

func TestFactoryGetters(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	assert.Equal(t, config, factory.GetConfig())
	assert.Equal(t, gcpClient, factory.GetGCPClient())
}

func TestCreateInstanceWorkflow_NoDisks(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	instance := &computepb.Instance{
		Name:   proto.String("test-instance"),
		Status: proto.String("TERMINATED"),
		Disks:  nil, // No disks
	}
	
	nodes, deps, err := factory.CreateInstanceWorkflow(instance)
	
	assert.NoError(t, err)
	assert.Empty(t, nodes)
	assert.Empty(t, deps)
}

func TestCreateInstanceWorkflow_OnlyBootDisk(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	instance := &computepb.Instance{
		Name:   proto.String("test-instance"),
		Status: proto.String("TERMINATED"),
		Disks: []*computepb.AttachedDisk{
			{
				Source:     proto.String("projects/test-project/zones/us-central1-a/disks/boot-disk"),
				DeviceName: proto.String("persistent-disk-0"),
				Boot:       proto.Bool(true),
			},
		},
	}
	
	nodes, deps, err := factory.CreateInstanceWorkflow(instance)
	
	assert.NoError(t, err)
	assert.Empty(t, nodes) // No non-boot disks to migrate
	assert.Empty(t, deps)
}

func TestTaskFactoryIntegration(t *testing.T) {
	// Integration test that verifies the factory creates valid tasks
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
		Concurrency:    5,
		RetainName:     false,
	}
	
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	// Test creating various task types
	discoveryNode := factory.CreateDiscoveryTask("instances")
	snapshotNode := factory.CreateSnapshotTask("test-disk", "test-snapshot")
	stateNode := factory.CreateInstanceStateTask("test-instance", "stop")
	cleanupNode := factory.CreateCleanupTask("snapshot", "test-snapshot")
	
	// Verify all tasks are created correctly
	tasks := []string{discoveryNode.ID(), snapshotNode.ID(), stateNode.ID(), cleanupNode.ID()}
	
	for _, taskID := range tasks {
		assert.NotEmpty(t, taskID)
	}
	
	// Verify task types are correct
	assert.Equal(t, "Discovery", dag.GetTaskType(discoveryNode.GetTask()))
	assert.Equal(t, "Snapshot", dag.GetTaskType(snapshotNode.GetTask()))
	assert.Equal(t, "InstanceState", dag.GetTaskType(stateNode.GetTask()))
	assert.Equal(t, "Cleanup", dag.GetTaskType(cleanupNode.GetTask()))
}

func TestTaskFactoryConfigurationMapping(t *testing.T) {
	// Test that factory correctly maps configuration to tasks
	config := &migrator.Config{
		ProjectID:      "test-project-123",
		TargetDiskType: "pd-extreme",
		Zone:           "us-west1-b",
		RetainName:     true,
	}
	
	gcpClient := &gcp.Clients{}
	factory := NewTaskFactory(config, gcpClient)
	
	// Verify configuration is properly accessible
	assert.Equal(t, "test-project-123", factory.GetConfig().ProjectID)
	assert.Equal(t, "pd-extreme", factory.GetConfig().TargetDiskType)
	assert.Equal(t, "us-west1-b", factory.GetConfig().Zone)
	assert.True(t, factory.GetConfig().RetainName)
}