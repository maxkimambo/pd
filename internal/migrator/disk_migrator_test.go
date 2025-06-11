package migrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

type mockComputeClientForMigrator struct {
	DetachDiskFunc func(ctx context.Context, projectID, zone, instanceName, deviceName string) error
	AttachDiskFunc func(ctx context.Context, projectID, zone, instanceName, diskName, deviceName string) error

	DetachDiskErr error
	AttachDiskErr error

	DetachDiskCalled bool
	AttachDiskCalled bool

	LastDetachProjectID    string
	LastDetachZone         string
	LastDetachInstanceName string
	LastDetachDeviceName   string

	LastAttachProjectID    string
	LastAttachZone         string
	LastAttachInstanceName string
	LastAttachDiskName     string
	LastAttachDeviceName   string
}

func (m *mockComputeClientForMigrator) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) error {
	m.DetachDiskCalled = true
	m.LastDetachProjectID = projectID
	m.LastDetachZone = zone
	m.LastDetachInstanceName = instanceName
	m.LastDetachDeviceName = deviceName

	if m.DetachDiskFunc != nil {
		return m.DetachDiskFunc(ctx, projectID, zone, instanceName, deviceName)
	}
	return m.DetachDiskErr
}

func (m *mockComputeClientForMigrator) AttachDisk(ctx context.Context, projectID, zone, instanceName, diskName, deviceName string) error {
	m.AttachDiskCalled = true
	m.LastAttachProjectID = projectID
	m.LastAttachZone = zone
	m.LastAttachInstanceName = instanceName
	m.LastAttachDiskName = diskName
	m.LastAttachDeviceName = deviceName

	if m.AttachDiskFunc != nil {
		return m.AttachDiskFunc(ctx, projectID, zone, instanceName, diskName, deviceName)
	}
	return m.AttachDiskErr
}

// Implement other required interface methods as no-ops for testing
func (m *mockComputeClientForMigrator) StartInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClientForMigrator) StopInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClientForMigrator) ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error) {
	return nil, nil
}
func (m *mockComputeClientForMigrator) AggregatedListInstances(ctx context.Context, projectID string) ([]*computepb.Instance, error) {
	return nil, nil
}
func (m *mockComputeClientForMigrator) GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error) {
	return nil, nil
}
func (m *mockComputeClientForMigrator) InstanceIsRunning(ctx context.Context, instance *computepb.Instance) bool {
	return true
}
func (m *mockComputeClientForMigrator) GetInstanceDisks(ctx context.Context, projectID, zone, instanceName string) ([]*computepb.AttachedDisk, error) {
	return nil, nil
}
func (m *mockComputeClientForMigrator) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClientForMigrator) Close() error {
	return nil
}

type mockDiskClientForMigrator struct {
	CreateNewDiskFromSnapshotFunc func(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, size int64, iops int64, throughput int64, storagePoolID string) error

	CreateNewDiskFromSnapshotErr error

	CreateNewDiskFromSnapshotCalled bool

	LastCreateProjectID      string
	LastCreateZone           string
	LastCreateNewDiskName    string
	LastCreateTargetDiskType string
	LastCreateSnapshotSource string
	LastCreateLabels         map[string]string
	LastCreateSize           int64
	LastCreateIops           int64
	LastCreateThroughput     int64
	LastCreateStoragePoolID  string
}

func (m *mockDiskClientForMigrator) CreateNewDiskFromSnapshot(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, size int64, iops int64, throughput int64, storagePoolID string) error {
	m.CreateNewDiskFromSnapshotCalled = true
	m.LastCreateProjectID = projectID
	m.LastCreateZone = zone
	m.LastCreateNewDiskName = newDiskName
	m.LastCreateTargetDiskType = targetDiskType
	m.LastCreateSnapshotSource = snapshotSource
	m.LastCreateLabels = labels
	m.LastCreateSize = size
	m.LastCreateIops = iops
	m.LastCreateThroughput = throughput
	m.LastCreateStoragePoolID = storagePoolID

	if m.CreateNewDiskFromSnapshotFunc != nil {
		return m.CreateNewDiskFromSnapshotFunc(ctx, projectID, zone, newDiskName, targetDiskType, snapshotSource, labels, size, iops, throughput, storagePoolID)
	}
	return m.CreateNewDiskFromSnapshotErr
}

// Implement other required interface methods as no-ops for testing
func (m *mockDiskClientForMigrator) GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	return nil, nil
}
func (m *mockDiskClientForMigrator) ListDetachedDisks(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error) {
	return nil, nil
}
func (m *mockDiskClientForMigrator) UpdateDiskLabel(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error {
	return nil
}
func (m *mockDiskClientForMigrator) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	return nil
}
func (m *mockDiskClientForMigrator) Close() error {
	return nil
}

type mockSnapshotClientForMigrator struct {
	CreateSnapshotFunc func(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *gcp.SnapshotKmsParams, labels map[string]string) error
	GetSnapshotFunc    func(ctx context.Context, projectID, snapshotName string) (*computepb.Snapshot, error)

	CreateSnapshotErr error
	GetSnapshotResp   *computepb.Snapshot
	GetSnapshotErr    error

	CreateSnapshotCalled bool
	GetSnapshotCalled    bool

	LastCreateProjectID    string
	LastCreateZone         string
	LastCreateDiskName     string
	LastCreateSnapshotName string
	LastCreateKmsParams    *gcp.SnapshotKmsParams
	LastCreateLabels       map[string]string

	LastGetProjectID    string
	LastGetSnapshotName string
}

func (m *mockSnapshotClientForMigrator) CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *gcp.SnapshotKmsParams, labels map[string]string) error {
	m.CreateSnapshotCalled = true
	m.LastCreateProjectID = projectID
	m.LastCreateZone = zone
	m.LastCreateDiskName = diskName
	m.LastCreateSnapshotName = snapshotName
	m.LastCreateKmsParams = kmsParams
	m.LastCreateLabels = labels

	if m.CreateSnapshotFunc != nil {
		return m.CreateSnapshotFunc(ctx, projectID, zone, diskName, snapshotName, kmsParams, labels)
	}
	return m.CreateSnapshotErr
}

func (m *mockSnapshotClientForMigrator) GetSnapshot(ctx context.Context, projectID, snapshotName string) (*computepb.Snapshot, error) {
	m.GetSnapshotCalled = true
	m.LastGetProjectID = projectID
	m.LastGetSnapshotName = snapshotName

	if m.GetSnapshotFunc != nil {
		return m.GetSnapshotFunc(ctx, projectID, snapshotName)
	}
	return m.GetSnapshotResp, m.GetSnapshotErr
}

// Implement other required interface methods as no-ops for testing
func (m *mockSnapshotClientForMigrator) DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error {
	return nil
}
func (m *mockSnapshotClientForMigrator) ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
	return nil, nil
}
func (m *mockSnapshotClientForMigrator) Close() error {
	return nil
}

func TestDiskMigrator_MigrateInstanceDisks(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"

	config := &Config{
		ProjectID:      projectID,
		TargetDiskType: "pd-ssd",
		Iops:           1000,
		Throughput:     200,
		StoragePoolId:  "",
	}

	t.Run("Success - Non-boot disk migration", func(t *testing.T) {
		instance := &computepb.Instance{
			Name: proto.String("test-instance"),
			Zone: proto.String("projects/test-project/zones/us-west1-a"),
		}

		disk := &computepb.Disk{
			Name:   proto.String("test-disk"),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Type:   proto.String("pd-standard"),
			SizeGb: proto.Int64(100),
		}

		attachedDisk := &computepb.AttachedDisk{
			Source:     proto.String("projects/test-project/zones/us-west1-a/disks/test-disk"),
			Boot:       proto.Bool(false),
			DeviceName: proto.String("test-device"),
		}

		diskInfo := &AttachedDiskInfo{
			AttachedDisk: attachedDisk,
			DiskDetails:  disk,
			IsBoot:       false,
		}

		migration := &InstanceMigration{
			Instance:     instance,
			InitialState: InstanceStateRunning,
			Status:       MigrationStatusPending,
			Disks:        []*AttachedDiskInfo{diskInfo},
			Results:      []DiskMigrationResult{},
			Errors:       []MigrationError{},
		}

		computeClient := &mockComputeClientForMigrator{}
		diskClient := &mockDiskClientForMigrator{}
		snapshotClient := &mockSnapshotClientForMigrator{
			GetSnapshotResp: &computepb.Snapshot{
				Name:   proto.String("test-snapshot"),
				Status: proto.String("READY"),
			},
		}

		migrator := NewDiskMigrator(computeClient, diskClient, snapshotClient, config)
		err := migrator.MigrateInstanceDisks(ctx, migration)

		assert.NoError(t, err)
		assert.Equal(t, MigrationStatusCompleted, migration.Status)
		assert.Len(t, migration.Results, 1)
		assert.True(t, migration.Results[0].Success)

		// Verify all operations were called
		assert.True(t, computeClient.DetachDiskCalled)
		assert.True(t, snapshotClient.CreateSnapshotCalled)
		assert.True(t, diskClient.CreateNewDiskFromSnapshotCalled)
		assert.True(t, computeClient.AttachDiskCalled)
	})

	t.Run("Boot disk skipped", func(t *testing.T) {
		instance := &computepb.Instance{
			Name: proto.String("test-instance"),
			Zone: proto.String("projects/test-project/zones/us-west1-a"),
		}

		disk := &computepb.Disk{
			Name:   proto.String("boot-disk"),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Type:   proto.String("pd-standard"),
			SizeGb: proto.Int64(100),
		}

		attachedDisk := &computepb.AttachedDisk{
			Source:     proto.String("projects/test-project/zones/us-west1-a/disks/boot-disk"),
			Boot:       proto.Bool(true),
			DeviceName: proto.String("boot-device"),
		}

		diskInfo := &AttachedDiskInfo{
			AttachedDisk: attachedDisk,
			DiskDetails:  disk,
			IsBoot:       true,
		}

		migration := &InstanceMigration{
			Instance:     instance,
			InitialState: InstanceStateRunning,
			Status:       MigrationStatusPending,
			Disks:        []*AttachedDiskInfo{diskInfo},
			Results:      []DiskMigrationResult{},
			Errors:       []MigrationError{},
		}

		computeClient := &mockComputeClientForMigrator{}
		diskClient := &mockDiskClientForMigrator{}
		snapshotClient := &mockSnapshotClientForMigrator{}

		migrator := NewDiskMigrator(computeClient, diskClient, snapshotClient, config)
		err := migrator.MigrateInstanceDisks(ctx, migration)

		assert.NoError(t, err)
		assert.Equal(t, MigrationStatusCompleted, migration.Status)
		assert.Len(t, migration.Results, 1)
		assert.True(t, migration.Results[0].Success)
		assert.Contains(t, migration.Results[0].Error.Error(), "boot disk migration not supported")

		// Verify no operations were called for boot disk
		assert.False(t, computeClient.DetachDiskCalled)
		assert.False(t, snapshotClient.CreateSnapshotCalled)
		assert.False(t, diskClient.CreateNewDiskFromSnapshotCalled)
		assert.False(t, computeClient.AttachDiskCalled)
	})

	t.Run("Detach disk error", func(t *testing.T) {
		instance := &computepb.Instance{
			Name: proto.String("test-instance"),
			Zone: proto.String("projects/test-project/zones/us-west1-a"),
		}

		disk := &computepb.Disk{
			Name:   proto.String("test-disk"),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Type:   proto.String("pd-standard"),
			SizeGb: proto.Int64(100),
		}

		attachedDisk := &computepb.AttachedDisk{
			Source:     proto.String("projects/test-project/zones/us-west1-a/disks/test-disk"),
			Boot:       proto.Bool(false),
			DeviceName: proto.String("test-device"),
		}

		diskInfo := &AttachedDiskInfo{
			AttachedDisk: attachedDisk,
			DiskDetails:  disk,
			IsBoot:       false,
		}

		migration := &InstanceMigration{
			Instance:     instance,
			InitialState: InstanceStateRunning,
			Status:       MigrationStatusPending,
			Disks:        []*AttachedDiskInfo{diskInfo},
			Results:      []DiskMigrationResult{},
			Errors:       []MigrationError{},
		}

		computeClient := &mockComputeClientForMigrator{
			DetachDiskErr: errors.New("detach failed"),
		}
		diskClient := &mockDiskClientForMigrator{}
		snapshotClient := &mockSnapshotClientForMigrator{}

		migrator := NewDiskMigrator(computeClient, diskClient, snapshotClient, config)
		err := migrator.MigrateInstanceDisks(ctx, migration)

		assert.NoError(t, err)
		assert.Equal(t, MigrationStatusFailed, migration.Status)
		assert.Len(t, migration.Results, 1)
		assert.False(t, migration.Results[0].Success)
		assert.Contains(t, migration.Results[0].Error.Error(), "failed to detach disk")

		// Verify detach was called but others were not
		assert.True(t, computeClient.DetachDiskCalled)
		assert.False(t, snapshotClient.CreateSnapshotCalled)
		assert.False(t, diskClient.CreateNewDiskFromSnapshotCalled)
		assert.False(t, computeClient.AttachDiskCalled)
	})

	t.Run("Snapshot creation error", func(t *testing.T) {
		instance := &computepb.Instance{
			Name: proto.String("test-instance"),
			Zone: proto.String("projects/test-project/zones/us-west1-a"),
		}

		disk := &computepb.Disk{
			Name:   proto.String("test-disk"),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Type:   proto.String("pd-standard"),
			SizeGb: proto.Int64(100),
		}

		attachedDisk := &computepb.AttachedDisk{
			Source:     proto.String("projects/test-project/zones/us-west1-a/disks/test-disk"),
			Boot:       proto.Bool(false),
			DeviceName: proto.String("test-device"),
		}

		diskInfo := &AttachedDiskInfo{
			AttachedDisk: attachedDisk,
			DiskDetails:  disk,
			IsBoot:       false,
		}

		migration := &InstanceMigration{
			Instance:     instance,
			InitialState: InstanceStateRunning,
			Status:       MigrationStatusPending,
			Disks:        []*AttachedDiskInfo{diskInfo},
			Results:      []DiskMigrationResult{},
			Errors:       []MigrationError{},
		}

		computeClient := &mockComputeClientForMigrator{}
		diskClient := &mockDiskClientForMigrator{}
		snapshotClient := &mockSnapshotClientForMigrator{
			CreateSnapshotErr: errors.New("snapshot creation failed"),
		}

		migrator := NewDiskMigrator(computeClient, diskClient, snapshotClient, config)
		err := migrator.MigrateInstanceDisks(ctx, migration)

		assert.NoError(t, err)
		assert.Equal(t, MigrationStatusFailed, migration.Status)
		assert.Len(t, migration.Results, 1)
		assert.False(t, migration.Results[0].Success)
		assert.Contains(t, migration.Results[0].Error.Error(), "failed to create snapshot")

		// Verify sequence stopped at snapshot creation
		assert.True(t, computeClient.DetachDiskCalled)
		assert.True(t, snapshotClient.CreateSnapshotCalled)
		assert.False(t, diskClient.CreateNewDiskFromSnapshotCalled)
		assert.False(t, computeClient.AttachDiskCalled)
	})
}

func TestDiskMigrator_WaitForSnapshotReady(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	snapshotName := "test-snapshot"

	config := &Config{ProjectID: projectID}

	t.Run("Snapshot ready immediately", func(t *testing.T) {
		snapshotClient := &mockSnapshotClientForMigrator{
			GetSnapshotResp: &computepb.Snapshot{
				Name:   proto.String(snapshotName),
				Status: proto.String("READY"),
			},
		}

		migrator := NewDiskMigrator(nil, nil, snapshotClient, config)
		err := migrator.waitForSnapshotReady(ctx, projectID, snapshotName)

		assert.NoError(t, err)
		assert.True(t, snapshotClient.GetSnapshotCalled)
	})

	t.Run("Snapshot becomes ready after wait", func(t *testing.T) {
		callCount := 0
		snapshotClient := &mockSnapshotClientForMigrator{
			GetSnapshotFunc: func(ctx context.Context, projectID, snapshotName string) (*computepb.Snapshot, error) {
				callCount++
				if callCount == 1 {
					return &computepb.Snapshot{
						Name:   proto.String(snapshotName),
						Status: proto.String("CREATING"),
					}, nil
				}
				return &computepb.Snapshot{
					Name:   proto.String(snapshotName),
					Status: proto.String("READY"),
				}, nil
			},
		}

		migrator := NewDiskMigrator(nil, nil, snapshotClient, config)
		
		// Use a shorter context timeout for testing
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		
		err := migrator.waitForSnapshotReady(ctxWithTimeout, projectID, snapshotName)

		assert.NoError(t, err)
		assert.True(t, snapshotClient.GetSnapshotCalled)
		assert.Equal(t, 2, callCount)
	})

	t.Run("Context cancelled", func(t *testing.T) {
		snapshotClient := &mockSnapshotClientForMigrator{
			GetSnapshotResp: &computepb.Snapshot{
				Name:   proto.String(snapshotName),
				Status: proto.String("CREATING"),
			},
		}

		migrator := NewDiskMigrator(nil, nil, snapshotClient, config)
		
		// Cancel context immediately
		ctxWithCancel, cancel := context.WithCancel(ctx)
		cancel()
		
		err := migrator.waitForSnapshotReady(ctxWithCancel, projectID, snapshotName)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestExtractProjectFromSelfLink(t *testing.T) {
	tests := []struct {
		name     string
		selfLink string
		expected string
	}{
		{
			name:     "Valid GCP self link",
			selfLink: "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-west1-a/instances/my-instance",
			expected: "my-project",
		},
		{
			name:     "Another valid self link",
			selfLink: "https://www.googleapis.com/compute/v1/projects/test-project-123/regions/us-west1/disks/my-disk",
			expected: "test-project-123",
		},
		{
			name:     "Empty string",
			selfLink: "",
			expected: "",
		},
		{
			name:     "Invalid format",
			selfLink: "invalid-link",
			expected: "",
		},
		{
			name:     "Missing projects section",
			selfLink: "https://www.googleapis.com/compute/v1/zones/us-west1-a/instances/my-instance",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProjectFromSelfLink(tt.selfLink)
			assert.Equal(t, tt.expected, result)
		})
	}
}