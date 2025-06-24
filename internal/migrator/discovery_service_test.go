package migrator

import (
	"context"
	"errors"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

type mockComputeClient struct {
	GetInstanceFunc         func(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error)
	ListInstancesInZoneFunc func(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error)

	GetInstanceResp         *computepb.Instance
	GetInstanceErr          error
	ListInstancesInZoneResp []*computepb.Instance
	ListInstancesInZoneErr  error

	GetInstanceCalled         bool
	ListInstancesInZoneCalled bool

	LastGetProjectID    string
	LastGetZone         string
	LastGetInstanceName string
	LastListProjectID   string
	LastListZone        string
}

func (m *mockComputeClient) GetInstance(ctx context.Context, projectID, zone, instanceName string) (*computepb.Instance, error) {
	m.GetInstanceCalled = true
	m.LastGetProjectID = projectID
	m.LastGetZone = zone
	m.LastGetInstanceName = instanceName

	if m.GetInstanceFunc != nil {
		return m.GetInstanceFunc(ctx, projectID, zone, instanceName)
	}
	return m.GetInstanceResp, m.GetInstanceErr
}

func (m *mockComputeClient) ListInstancesInZone(ctx context.Context, projectID, zone string) ([]*computepb.Instance, error) {
	m.ListInstancesInZoneCalled = true
	m.LastListProjectID = projectID
	m.LastListZone = zone

	if m.ListInstancesInZoneFunc != nil {
		return m.ListInstancesInZoneFunc(ctx, projectID, zone)
	}
	return m.ListInstancesInZoneResp, m.ListInstancesInZoneErr
}

// Implement other required interface methods as no-ops for testing
func (m *mockComputeClient) StartInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClient) StopInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClient) AggregatedListInstances(ctx context.Context, projectID string) ([]*computepb.Instance, error) {
	return nil, nil
}
func (m *mockComputeClient) InstanceIsRunning(ctx context.Context, instance *computepb.Instance) bool {
	return true
}
func (m *mockComputeClient) GetInstanceDisks(ctx context.Context, projectID, zone, instanceName string) ([]*computepb.AttachedDisk, error) {
	return nil, nil
}
func (m *mockComputeClient) DeleteInstance(ctx context.Context, projectID, zone, instanceName string) error {
	return nil
}
func (m *mockComputeClient) AttachDisk(ctx context.Context, projectID, zone, instanceName, diskName, deviceName string) error {
	return nil
}
func (m *mockComputeClient) DetachDisk(ctx context.Context, projectID, zone, instanceName, deviceName string) error {
	return nil
}
func (m *mockComputeClient) Close() error {
	return nil
}

type mockDiskClientForDiscovery struct {
	GetDiskFunc func(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error)

	GetDiskResp *computepb.Disk
	GetDiskErr  error

	GetDiskCalled bool

	LastGetProjectID string
	LastGetZone      string
	LastGetDiskName  string
}

func (m *mockDiskClientForDiscovery) GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	m.GetDiskCalled = true
	m.LastGetProjectID = projectID
	m.LastGetZone = zone
	m.LastGetDiskName = diskName

	if m.GetDiskFunc != nil {
		return m.GetDiskFunc(ctx, projectID, zone, diskName)
	}
	return m.GetDiskResp, m.GetDiskErr
}

// Implement other required interface methods as no-ops for testing
func (m *mockDiskClientForDiscovery) ListDetachedDisks(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error) {
	return nil, nil
}
func (m *mockDiskClientForDiscovery) CreateNewDiskFromSnapshot(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, size int64, iops int64, throughput int64, storagePoolID string) error {
	return nil
}
func (m *mockDiskClientForDiscovery) UpdateDiskLabel(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error {
	return nil
}
func (m *mockDiskClientForDiscovery) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	return nil
}
func (m *mockDiskClientForDiscovery) Close() error {
	return nil
}

func TestInstanceDiscovery_DiscoverByNames(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-west1-a"
	instanceName := "test-instance"

	config := &Config{
		ProjectID: projectID,
		Zone:      zone,
		Instances: []string{instanceName},
	}

	t.Run("Success", func(t *testing.T) {
		instance := &computepb.Instance{
			Name:   proto.String(instanceName),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Status: proto.String("RUNNING"),
			Disks: []*computepb.AttachedDisk{
				{
					Source: proto.String("projects/test-project/zones/us-west1-a/disks/test-disk"),
					Boot:   proto.Bool(true),
				},
			},
		}

		disk := &computepb.Disk{
			Name:   proto.String("test-disk"),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Type:   proto.String("pd-standard"),
			SizeGb: proto.Int64(100),
		}

		computeClient := &mockComputeClient{
			GetInstanceResp: instance,
		}
		diskClient := &mockDiskClientForDiscovery{
			GetDiskResp: disk,
		}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverByNames(ctx, config)

		assert.NoError(t, err)
		assert.Len(t, migrations, 1)
		assert.True(t, computeClient.GetInstanceCalled)
		assert.True(t, diskClient.GetDiskCalled)

		migration := migrations[0]
		assert.Equal(t, instance, migration.Instance)
		assert.Equal(t, InstanceStateRunning, migration.InitialState)
		assert.Equal(t, MigrationStatusPending, migration.Status)
		assert.Len(t, migration.Disks, 1)
		assert.True(t, migration.Disks[0].IsBoot)
		assert.Equal(t, disk, migration.Disks[0].DiskDetails)
	})

	t.Run("Instance Not Found", func(t *testing.T) {
		computeClient := &mockComputeClient{
			GetInstanceErr: errors.New("instance not found"),
		}
		diskClient := &mockDiskClientForDiscovery{}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverByNames(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, migrations)
		assert.Contains(t, err.Error(), "failed to get instance")
		assert.True(t, computeClient.GetInstanceCalled)
		assert.False(t, diskClient.GetDiskCalled)
	})

	t.Run("Disk Not Found", func(t *testing.T) {
		instance := &computepb.Instance{
			Name:   proto.String(instanceName),
			Zone:   proto.String("projects/test-project/zones/us-west1-a"),
			Status: proto.String("RUNNING"),
			Disks: []*computepb.AttachedDisk{
				{
					Source: proto.String("projects/test-project/zones/us-west1-a/disks/missing-disk"),
					Boot:   proto.Bool(false),
				},
			},
		}

		computeClient := &mockComputeClient{
			GetInstanceResp: instance,
		}
		diskClient := &mockDiskClientForDiscovery{
			GetDiskErr: errors.New("disk not found"),
		}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverByNames(ctx, config)

		// Should succeed but with errors recorded in the migration
		assert.NoError(t, err)
		assert.Len(t, migrations, 1)
		assert.True(t, computeClient.GetInstanceCalled)
		assert.True(t, diskClient.GetDiskCalled)

		migration := migrations[0]
		assert.Len(t, migration.Errors, 1)
		assert.Equal(t, ErrorTypePermanent, migration.Errors[0].Type)
		assert.Equal(t, PhaseDiscovery, migration.Errors[0].Phase)
		assert.Contains(t, migration.Errors[0].Target, "disk:missing-disk")
	})
}

func TestInstanceDiscovery_DiscoverByZone(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-west1-a"

	config := &Config{
		ProjectID: projectID,
		Zone:      zone,
	}

	t.Run("Success", func(t *testing.T) {
		instances := []*computepb.Instance{
			{
				Name:   proto.String("instance-1"),
				Zone:   proto.String("projects/test-project/zones/us-west1-a"),
				Status: proto.String("RUNNING"),
				Disks:  []*computepb.AttachedDisk{},
			},
			{
				Name:   proto.String("instance-2"),
				Zone:   proto.String("projects/test-project/zones/us-west1-a"),
				Status: proto.String("TERMINATED"),
				Disks:  []*computepb.AttachedDisk{},
			},
		}

		computeClient := &mockComputeClient{
			ListInstancesInZoneResp: instances,
		}
		diskClient := &mockDiskClientForDiscovery{}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverByZone(ctx, config)

		assert.NoError(t, err)
		assert.Len(t, migrations, 2)
		assert.True(t, computeClient.ListInstancesInZoneCalled)
		assert.Equal(t, projectID, computeClient.LastListProjectID)
		assert.Equal(t, zone, computeClient.LastListZone)

		assert.Equal(t, InstanceStateRunning, migrations[0].InitialState)
		assert.Equal(t, InstanceStateStopped, migrations[1].InitialState)
	})

	t.Run("List Error", func(t *testing.T) {
		computeClient := &mockComputeClient{
			ListInstancesInZoneErr: errors.New("list failed"),
		}
		diskClient := &mockDiskClientForDiscovery{}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverByZone(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, migrations)
		assert.Contains(t, err.Error(), "failed to list instances")
		assert.True(t, computeClient.ListInstancesInZoneCalled)
	})
}

func TestInstanceDiscovery_DiscoverInstances(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	discovery := NewInstanceDiscovery(&mockComputeClient{}, &mockDiskClientForDiscovery{})

	t.Run("No Discovery Criteria", func(t *testing.T) {
		config := &Config{
			ProjectID: "test-project",
		}

		migrations, err := discovery.DiscoverInstances(ctx, config)

		assert.Error(t, err)
		assert.Nil(t, migrations)
		assert.Contains(t, err.Error(), "no discovery criteria provided")
	})

	t.Run("Discover By Names", func(t *testing.T) {
		config := &Config{
			ProjectID: "test-project",
			Zone:      "us-west1-a",
			Instances: []string{"test-instance"},
		}

		computeClient := &mockComputeClient{
			GetInstanceResp: &computepb.Instance{
				Name:   proto.String("test-instance"),
				Zone:   proto.String("projects/test-project/zones/us-west1-a"),
				Status: proto.String("RUNNING"),
				Disks:  []*computepb.AttachedDisk{},
			},
		}
		diskClient := &mockDiskClientForDiscovery{}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverInstances(ctx, config)

		assert.NoError(t, err)
		assert.Len(t, migrations, 1)
		assert.True(t, computeClient.GetInstanceCalled)
	})

	t.Run("Discover By Zone", func(t *testing.T) {
		config := &Config{
			ProjectID: "test-project",
			Zone:      "us-west1-a",
		}

		computeClient := &mockComputeClient{
			ListInstancesInZoneResp: []*computepb.Instance{
				{
					Name:   proto.String("instance-1"),
					Zone:   proto.String("projects/test-project/zones/us-west1-a"),
					Status: proto.String("RUNNING"),
					Disks:  []*computepb.AttachedDisk{},
				},
			},
		}
		diskClient := &mockDiskClientForDiscovery{}

		discovery := NewInstanceDiscovery(computeClient, diskClient)
		migrations, err := discovery.DiscoverInstances(ctx, config)

		assert.NoError(t, err)
		assert.Len(t, migrations, 1)
		assert.True(t, computeClient.ListInstancesInZoneCalled)
	})
}

func TestGetInstanceState(t *testing.T) {
	tests := []struct {
		name           string
		instanceStatus string
		expectedState  InstanceState
	}{
		{"Running", "RUNNING", InstanceStateRunning},
		{"Stopped", "TERMINATED", InstanceStateStopped},
		{"Suspended", "SUSPENDED", InstanceStateSuspended},
		{"Unknown", "PROVISIONING", InstanceStateUnknown},
		{"Empty", "", InstanceStateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &computepb.Instance{
				Status: proto.String(tt.instanceStatus),
			}
			state := getInstanceState(instance)
			assert.Equal(t, tt.expectedState, state)
		})
	}
}

func TestExtractDiskNameFromSource(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "Full GCP URL",
			source:   "projects/my-project/zones/us-west1-a/disks/my-disk",
			expected: "my-disk",
		},
		{
			name:     "Simple name",
			source:   "simple-disk",
			expected: "simple-disk",
		},
		{
			name:     "Empty string",
			source:   "",
			expected: "",
		},
		{
			name:     "URL with trailing slash",
			source:   "projects/my-project/zones/us-west1-a/disks/my-disk/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDiskNameFromSource(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}
