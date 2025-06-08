package gcp

import (
	"context"
	"errors"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

type mockSnapshotClient struct {
	SnapshotClientInterface
	CreateSnapshotFunc       func(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error
	DeleteSnapshotFunc       func(ctx context.Context, projectID, snapshotName string) error
	ListSnapshotsByLabelFunc func(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error)

	CreateSnapshotErr error
	DeleteSnapshotErr error
	ListSnapshotsResp []*computepb.Snapshot
	ListSnapshotsErr  error

	CreateSnapshotCalled bool
	DeleteSnapshotCalled bool
	ListSnapshotsCalled  bool

	// Captured parameters for verification
	LastCreateProjectID    string
	LastCreateZone         string
	LastCreateDiskName     string
	LastCreateSnapshotName string
	LastCreateKmsParams    *SnapshotKmsParams
	LastCreateLabels       map[string]string

	LastDeleteProjectID    string
	LastDeleteSnapshotName string

	LastListProjectID  string
	LastListLabelKey   string
	LastListLabelValue string
}

func (m *mockSnapshotClient) CreateSnapshot(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error {
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

func (m *mockSnapshotClient) DeleteSnapshot(ctx context.Context, projectID, snapshotName string) error {
	m.DeleteSnapshotCalled = true
	m.LastDeleteProjectID = projectID
	m.LastDeleteSnapshotName = snapshotName

	if m.DeleteSnapshotFunc != nil {
		return m.DeleteSnapshotFunc(ctx, projectID, snapshotName)
	}
	return m.DeleteSnapshotErr
}

func (m *mockSnapshotClient) ListSnapshotsByLabel(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
	m.ListSnapshotsCalled = true
	m.LastListProjectID = projectID
	m.LastListLabelKey = labelKey
	m.LastListLabelValue = labelValue

	if m.ListSnapshotsByLabelFunc != nil {
		return m.ListSnapshotsByLabelFunc(ctx, projectID, labelKey, labelValue)
	}
	return m.ListSnapshotsResp, m.ListSnapshotsErr
}

func (m *mockSnapshotClient) Close() error {
	return nil
}

func TestCreateSnapshot(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-proj"
	zone := "us-west1-a"
	diskName := "source-disk"
	snapshotName := "new-snap"

	t.Run("Success - No KMS, No extra labels", func(t *testing.T) {
		mockClient := &mockSnapshotClient{}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, nil, nil)

		assert.NoError(t, err)
		assert.True(t, mockClient.CreateSnapshotCalled)
		assert.Equal(t, projectID, mockClient.LastCreateProjectID)
		assert.Equal(t, zone, mockClient.LastCreateZone)
		assert.Equal(t, diskName, mockClient.LastCreateDiskName)
		assert.Equal(t, snapshotName, mockClient.LastCreateSnapshotName)
		assert.Nil(t, mockClient.LastCreateKmsParams)
		assert.Nil(t, mockClient.LastCreateLabels)
	})

	t.Run("Success - With KMS, With extra labels", func(t *testing.T) {
		kmsParams := &SnapshotKmsParams{
			KmsKey:      "my-key",
			KmsKeyRing:  "my-ring",
			KmsLocation: "global",
			KmsProject:  "kms-proj",
		}
		initialLabels := map[string]string{"user": "test"}

		mockClient := &mockSnapshotClient{}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, kmsParams, initialLabels)

		assert.NoError(t, err)
		assert.True(t, mockClient.CreateSnapshotCalled)
		assert.Equal(t, projectID, mockClient.LastCreateProjectID)
		assert.Equal(t, zone, mockClient.LastCreateZone)
		assert.Equal(t, diskName, mockClient.LastCreateDiskName)
		assert.Equal(t, snapshotName, mockClient.LastCreateSnapshotName)
		assert.Equal(t, kmsParams, mockClient.LastCreateKmsParams)
		assert.Equal(t, initialLabels, mockClient.LastCreateLabels)
	})

	t.Run("Success - With KMS, Default KMS project", func(t *testing.T) {
		kmsParams := &SnapshotKmsParams{
			KmsKey:      "my-key",
			KmsKeyRing:  "my-ring",
			KmsLocation: "global",
			// KmsProject is empty, should default to projectID
		}

		mockClient := &mockSnapshotClient{}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, kmsParams, nil)

		assert.NoError(t, err)
		assert.True(t, mockClient.CreateSnapshotCalled)
		assert.Equal(t, kmsParams, mockClient.LastCreateKmsParams)
		assert.Equal(t, projectID, mockClient.LastCreateProjectID)
	})

	t.Run("CreateSnapshot Error", func(t *testing.T) {
		expectedErr := errors.New("snapshot creation failed")
		mockClient := &mockSnapshotClient{
			CreateSnapshotErr: expectedErr,
		}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, nil, nil)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockClient.CreateSnapshotCalled)
	})

	t.Run("CreateSnapshot Custom Function", func(t *testing.T) {
		customErr := errors.New("custom create error")
		mockClient := &mockSnapshotClient{
			CreateSnapshotFunc: func(ctx context.Context, projectID, zone, diskName, snapshotName string, kmsParams *SnapshotKmsParams, labels map[string]string) error {
				return customErr
			},
		}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, nil, nil)

		assert.Error(t, err)
		assert.Equal(t, customErr, err)
		assert.True(t, mockClient.CreateSnapshotCalled)
	})
}

func TestDeleteSnapshot(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-proj"
	snapshotName := "snap-to-delete"

	t.Run("Success", func(t *testing.T) {
		mockClient := &mockSnapshotClient{}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.DeleteSnapshot(ctx, projectID, snapshotName)

		assert.NoError(t, err)
		assert.True(t, mockClient.DeleteSnapshotCalled)
		assert.Equal(t, projectID, mockClient.LastDeleteProjectID)
		assert.Equal(t, snapshotName, mockClient.LastDeleteSnapshotName)
	})

	t.Run("DeleteSnapshot Error", func(t *testing.T) {
		expectedErr := errors.New("snapshot delete failed")
		mockClient := &mockSnapshotClient{
			DeleteSnapshotErr: expectedErr,
		}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.DeleteSnapshot(ctx, projectID, snapshotName)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockClient.DeleteSnapshotCalled)
	})

	t.Run("DeleteSnapshot Custom Function", func(t *testing.T) {
		customErr := errors.New("custom delete error")
		mockClient := &mockSnapshotClient{
			DeleteSnapshotFunc: func(ctx context.Context, projectID, snapshotName string) error {
				return customErr
			},
		}
		clients := &Clients{SnapshotClient: mockClient}
		err := clients.SnapshotClient.DeleteSnapshot(ctx, projectID, snapshotName)

		assert.Error(t, err)
		assert.Equal(t, customErr, err)
		assert.True(t, mockClient.DeleteSnapshotCalled)
	})
}

func TestListSnapshotsByLabel(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-proj"
	labelKey := "env"
	labelValue := "prod"

	t.Run("Success - Found snapshots", func(t *testing.T) {
		expectedSnapshots := []*computepb.Snapshot{
			{
				Name: proto.String("snapshot-1"),
				Labels: map[string]string{
					"env":        "prod",
					"managed-by": "pd-migrate",
				},
			},
			{
				Name: proto.String("snapshot-2"),
				Labels: map[string]string{
					"env":        "prod",
					"managed-by": "pd-migrate",
				},
			},
		}

		mockClient := &mockSnapshotClient{
			ListSnapshotsResp: expectedSnapshots,
		}
		clients := &Clients{SnapshotClient: mockClient}
		snapshots, err := clients.SnapshotClient.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockClient.ListSnapshotsCalled)
		assert.Equal(t, projectID, mockClient.LastListProjectID)
		assert.Equal(t, labelKey, mockClient.LastListLabelKey)
		assert.Equal(t, labelValue, mockClient.LastListLabelValue)
		assert.Equal(t, expectedSnapshots, snapshots)
		assert.Len(t, snapshots, 2)
	})

	t.Run("Success - No snapshots found", func(t *testing.T) {
		mockClient := &mockSnapshotClient{
			ListSnapshotsResp: []*computepb.Snapshot{},
		}
		clients := &Clients{SnapshotClient: mockClient}
		snapshots, err := clients.SnapshotClient.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockClient.ListSnapshotsCalled)
		assert.Equal(t, projectID, mockClient.LastListProjectID)
		assert.Equal(t, labelKey, mockClient.LastListLabelKey)
		assert.Equal(t, labelValue, mockClient.LastListLabelValue)
		assert.NotNil(t, snapshots)
		assert.Len(t, snapshots, 0)
	})

	t.Run("ListSnapshotsByLabel Error", func(t *testing.T) {
		expectedErr := errors.New("list snapshots failed")
		mockClient := &mockSnapshotClient{
			ListSnapshotsErr: expectedErr,
		}
		clients := &Clients{SnapshotClient: mockClient}
		snapshots, err := clients.SnapshotClient.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockClient.ListSnapshotsCalled)
		assert.Nil(t, snapshots)
	})

	t.Run("ListSnapshotsByLabel Custom Function", func(t *testing.T) {
		customSnapshots := []*computepb.Snapshot{
			{
				Name:   proto.String("custom-snapshot"),
				Labels: map[string]string{"env": "prod"},
			},
		}
		mockClient := &mockSnapshotClient{
			ListSnapshotsByLabelFunc: func(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
				return customSnapshots, nil
			},
		}
		clients := &Clients{SnapshotClient: mockClient}
		snapshots, err := clients.SnapshotClient.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockClient.ListSnapshotsCalled)
		assert.Equal(t, customSnapshots, snapshots)
		assert.Len(t, snapshots, 1)
	})

	t.Run("ListSnapshotsByLabel Custom Function Error", func(t *testing.T) {
		customErr := errors.New("custom list error")
		mockClient := &mockSnapshotClient{
			ListSnapshotsByLabelFunc: func(ctx context.Context, projectID, labelKey, labelValue string) ([]*computepb.Snapshot, error) {
				return nil, customErr
			},
		}
		clients := &Clients{SnapshotClient: mockClient}
		snapshots, err := clients.SnapshotClient.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.Error(t, err)
		assert.Equal(t, customErr, err)
		assert.True(t, mockClient.ListSnapshotsCalled)
		assert.Nil(t, snapshots)
	})
}
