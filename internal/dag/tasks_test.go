package dag

import (
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestTaskCreation(t *testing.T) {
	gcpClient := &gcp.Clients{}
	config := &migrator.Config{
		ProjectID: "test-project",
	}

	disk := &computepb.Disk{
		Name:   proto.String("test-disk"),
		SizeGb: proto.Int64(100),
		Labels: map[string]string{"env": "test"},
	}

	// Test DiscoveryTask creation
	t.Run("DiscoveryTask", func(t *testing.T) {
		task := NewDiscoveryTask("disc-1", config, gcpClient, "disks")

		assert.Equal(t, "disc-1", task.GetID())
		assert.Equal(t, "Discovery", GetTaskType(task))
		assert.Equal(t, "Discover disks", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "disks", task.resourceType)
	})

	// Test SnapshotTask creation
	t.Run("SnapshotTask", func(t *testing.T) {
		task := NewSnapshotTask("snap-1", "test-project", "us-central1-a", "test-disk", "test-snapshot", gcpClient, config)

		assert.Equal(t, "snap-1", task.GetID())
		assert.Equal(t, "Snapshot", GetTaskType(task))
		assert.Equal(t, "Create snapshot test-snapshot of disk test-disk", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "test-project", task.projectID)
		assert.Equal(t, "us-central1-a", task.zone)
		assert.Equal(t, "test-disk", task.diskName)
		assert.Equal(t, "test-snapshot", task.snapshotName)
		assert.False(t, task.created)
	})

	// Test InstanceStateTask creation
	t.Run("InstanceStateTask", func(t *testing.T) {
		task := NewInstanceStateTask("inst-1", "test-project", "us-central1-a", "test-instance", "stop", gcpClient)

		assert.Equal(t, "inst-1", task.GetID())
		assert.Equal(t, "InstanceState", GetTaskType(task))
		assert.Equal(t, "Stop instance test-instance", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "stop", task.action)
		assert.False(t, task.stateChanged)
	})

	// Test DiskMigrationTask creation with retain name
	t.Run("DiskMigrationTaskRetainName", func(t *testing.T) {
		config.RetainName = true
		task := NewDiskMigrationTask("mig-1", "test-project", "us-central1-a", "test-disk", "pd-ssd", "test-snapshot", gcpClient, config, disk)

		assert.Equal(t, "mig-1", task.GetID())
		assert.Equal(t, "DiskMigration", GetTaskType(task))
		assert.Equal(t, "Migrate disk test-disk to pd-ssd", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "test-disk", task.newDiskName) // Should retain name
		assert.False(t, task.migrated)
	})

	// Test DiskMigrationTask creation without retain name
	t.Run("DiskMigrationTaskNewName", func(t *testing.T) {
		config.RetainName = false
		task := NewDiskMigrationTask("mig-2", "test-project", "us-central1-a", "test-disk", "pd-ssd", "test-snapshot", gcpClient, config, disk)

		assert.Equal(t, "mig-2", task.GetID())
		assert.Equal(t, "DiskMigration", GetTaskType(task))
		assert.Equal(t, "Migrate disk test-disk to pd-ssd", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.NotEqual(t, "test-disk", task.newDiskName) // Should have new name
		assert.Contains(t, task.newDiskName, "test-disk") // Should contain original name
		assert.False(t, task.migrated)
	})

	// Test DiskAttachmentTask creation
	t.Run("DiskAttachmentTask", func(t *testing.T) {
		task := NewDiskAttachmentTask("attach-1", "test-project", "us-central1-a", "test-instance", "test-disk", "sdb", "attach", gcpClient)

		assert.Equal(t, "attach-1", task.GetID())
		assert.Equal(t, "DiskAttachment", GetTaskType(task))
		assert.Equal(t, "Attach disk test-disk to instance test-instance", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "attach", task.action)
		assert.False(t, task.executed)
	})

	// Test CleanupTask creation
	t.Run("CleanupTask", func(t *testing.T) {
		task := NewCleanupTask("cleanup-1", "test-project", "snapshot", "test-snapshot", gcpClient)

		assert.Equal(t, "cleanup-1", task.GetID())
		assert.Equal(t, "Cleanup", GetTaskType(task))
		assert.Equal(t, "Clean up snapshot test-snapshot", GetTaskDescription(task))
		// Validate() method removed from simplified interface
		assert.Equal(t, "snapshot", task.resourceType)
		assert.Equal(t, "test-snapshot", task.resourceID)
	})
}

func TestTaskInterface(t *testing.T) {
	gcpClient := &gcp.Clients{}
	config := &migrator.Config{}
	disk := &computepb.Disk{
		Name:   proto.String("test-disk"),
		SizeGb: proto.Int64(100),
	}

	// Test that all task types implement the Task interface
	var tasks []Task = []Task{
		NewDiscoveryTask("disc-1", config, gcpClient, "disks"),
		NewSnapshotTask("snap-1", "project", "zone", "disk", "snapshot", gcpClient, config),
		NewInstanceStateTask("inst-1", "project", "zone", "instance", "stop", gcpClient),
		NewDiskMigrationTask("mig-1", "project", "zone", "disk", "pd-ssd", "snapshot", gcpClient, config, disk),
		NewDiskAttachmentTask("attach-1", "project", "zone", "instance", "disk", "sdb", "attach", gcpClient),
		NewCleanupTask("cleanup-1", "project", "snapshot", "resource", gcpClient),
	}

	for _, task := range tasks {
		assert.NotEmpty(t, task.GetID())
		assert.NotEmpty(t, GetTaskType(task))
		assert.NotEmpty(t, GetTaskDescription(task))
		// Validate() method removed from simplified interface
	}
}

func TestDiscoveryTaskResourceTypes(t *testing.T) {
	gcpClient := &gcp.Clients{}
	config := &migrator.Config{}

	// Test valid resource types
	validTypes := []string{"disks", "instances"}
	for _, resourceType := range validTypes {
		task := NewDiscoveryTask("disc-1", config, gcpClient, resourceType)
		assert.Equal(t, resourceType, task.resourceType)
	}
}

func TestSnapshotTaskGetters(t *testing.T) {
	gcpClient := &gcp.Clients{}
	config := &migrator.Config{}

	task := NewSnapshotTask("snap-1", "test-project", "us-central1-a", "test-disk", "test-snapshot", gcpClient, config)

	assert.Equal(t, "test-snapshot", task.GetSnapshotName())
}

func TestDiskMigrationTaskGetters(t *testing.T) {
	gcpClient := &gcp.Clients{}
	config := &migrator.Config{RetainName: false}
	disk := &computepb.Disk{
		Name:   proto.String("test-disk"),
		SizeGb: proto.Int64(100),
	}

	task := NewDiskMigrationTask("mig-1", "test-project", "us-central1-a", "test-disk", "pd-ssd", "test-snapshot", gcpClient, config, disk)

	newDiskName := task.GetNewDiskName()
	assert.NotEmpty(t, newDiskName)
	assert.NotEqual(t, "test-disk", newDiskName) // Should be different due to RetainName = false
}
