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

type mockDiskClient struct {
	DiskClientInterface
	GetDiskFunc                   func(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error)
	ListDetachedDisksFunc         func(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error)
	CreateNewDiskFromSnapshotFunc func(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, iops int64, throughput int64, storagePoolID string) error
	UpdateDiskLabelFunc           func(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error
	DeleteDiskFunc                func(ctx context.Context, projectID, zone, diskName string) error

	GetDiskResp           *computepb.Disk
	GetDiskErr            error
	ListDetachedDisksResp []*computepb.Disk
	ListDetachedDisksErr  error
	CreateDiskErr         error
	UpdateLabelErr        error
	DeleteDiskErr         error

	GetDiskCalled           bool
	ListDetachedDisksCalled bool
	CreateDiskCalled        bool
	UpdateLabelCalled       bool
	DeleteDiskCalled        bool
}

func (m *mockDiskClient) GetDisk(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
	m.GetDiskCalled = true
	if m.GetDiskFunc != nil {
		return m.GetDiskFunc(ctx, projectID, zone, diskName)
	}
	return m.GetDiskResp, m.GetDiskErr
}

func (m *mockDiskClient) ListDetachedDisks(ctx context.Context, projectID string, location string, labelFilter string) ([]*computepb.Disk, error) {
	m.ListDetachedDisksCalled = true
	if m.ListDetachedDisksFunc != nil {
		return m.ListDetachedDisksFunc(ctx, projectID, location, labelFilter)
	}
	return m.ListDetachedDisksResp, m.ListDetachedDisksErr
}

func (m *mockDiskClient) CreateNewDiskFromSnapshot(ctx context.Context, projectID string, zone string, newDiskName string, targetDiskType string, snapshotSource string, labels map[string]string, iops int64, throughput int64, storagePoolID string) error {
	m.CreateDiskCalled = true
	if m.CreateNewDiskFromSnapshotFunc != nil {
		return m.CreateNewDiskFromSnapshotFunc(ctx, projectID, zone, newDiskName, targetDiskType, snapshotSource, labels, iops, throughput, storagePoolID)
	}
	return m.CreateDiskErr
}

func (m *mockDiskClient) UpdateDiskLabel(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error {
	m.UpdateLabelCalled = true
	if m.UpdateDiskLabelFunc != nil {
		return m.UpdateDiskLabelFunc(ctx, projectID, zone, diskName, labelKey, labelValue)
	}
	return m.UpdateLabelErr
}

func (m *mockDiskClient) DeleteDisk(ctx context.Context, projectID, zone, diskName string) error {
	m.DeleteDiskCalled = true
	if m.DeleteDiskFunc != nil {
		return m.DeleteDiskFunc(ctx, projectID, zone, diskName)
	}
	return m.DeleteDiskErr
}

func (m *mockDiskClient) Close() error {
	return nil
}

func TestListDetachedDisks(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	fullZonePath := "projects/" + projectID + "/zones/" + zone

	t.Run("Success - Found detached disks", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			ListDetachedDisksResp: []*computepb.Disk{},
			ListDetachedDisksErr:  nil,
		}

		disks, err := mockDiskClient.ListDetachedDisks(ctx, projectID, zone, "")

		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.True(t, mockDiskClient.ListDetachedDisksCalled)
		assert.Len(t, disks, 0)

		// Test with full zone path
		mockDiskClient.ListDetachedDisksCalled = false
		disks, err = mockDiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.True(t, mockDiskClient.ListDetachedDisksCalled)
		assert.Len(t, disks, 0)
	})

	t.Run("Success - No detached disks found", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			ListDetachedDisksResp: []*computepb.Disk{},
			ListDetachedDisksErr:  nil,
		}

		disks, err := mockDiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.True(t, mockDiskClient.ListDetachedDisksCalled)
		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.Len(t, disks, 0)
	})

	t.Run("API Error - List operation fails", func(t *testing.T) {
		expectedErr := errors.New("list failed")
		mockDiskClient := &mockDiskClient{
			ListDetachedDisksErr: expectedErr,
		}

		disks, err := mockDiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.True(t, mockDiskClient.ListDetachedDisksCalled)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, disks)
	})
}

func TestGetDisk(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	diskName := "test-disk"

	t.Run("Success", func(t *testing.T) {
		expectedDisk := &computepb.Disk{
			Name: proto.String(diskName),
			Zone: proto.String(zone),
		}
		mockDiskClient := &mockDiskClient{
			GetDiskResp: expectedDisk,
			GetDiskErr:  nil,
		}

		disk, err := mockDiskClient.GetDisk(ctx, projectID, zone, diskName)

		assert.NoError(t, err)
		assert.Equal(t, expectedDisk, disk)
		assert.True(t, mockDiskClient.GetDiskCalled)
	})

	t.Run("Get Disk Error", func(t *testing.T) {
		expectedErr := errors.New("get disk failed")
		mockDiskClient := &mockDiskClient{
			GetDiskErr: expectedErr,
		}

		disk, err := mockDiskClient.GetDisk(ctx, projectID, zone, diskName)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, disk)
		assert.True(t, mockDiskClient.GetDiskCalled)
	})

	t.Run("Get Disk with Custom Function", func(t *testing.T) {
		customDisk := &computepb.Disk{
			Name: proto.String("custom-disk"),
		}
		mockDiskClient := &mockDiskClient{
			GetDiskFunc: func(ctx context.Context, projectID, zone, diskName string) (*computepb.Disk, error) {
				return customDisk, nil
			},
		}

		disk, err := mockDiskClient.GetDisk(ctx, projectID, zone, diskName)

		assert.NoError(t, err)
		assert.Equal(t, customDisk, disk)
		assert.True(t, mockDiskClient.GetDiskCalled)
	})
}

func TestCreateNewDiskFromSnapshot(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	newDiskName := "new-disk"
	targetDiskType := "pd-ssd"
	snapshotName := "snap-1"
	labels := map[string]string{"env": "test"}
	storagePoolUrl := "projects/project/zones/zone/storagePools/storagePool"

	t.Run("Success", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			CreateDiskErr: nil,
		}

		err := mockDiskClient.CreateNewDiskFromSnapshot(ctx, projectID, zone, newDiskName, targetDiskType, snapshotName, labels, 0, 0, storagePoolUrl)

		assert.NoError(t, err)
		assert.True(t, mockDiskClient.CreateDiskCalled)
	})

	t.Run("Create Disk API Error", func(t *testing.T) {
		expectedErr := errors.New("create disk failed")
		mockDiskClient := &mockDiskClient{
			CreateDiskErr: expectedErr,
		}

		err := mockDiskClient.CreateNewDiskFromSnapshot(ctx, projectID, zone, newDiskName, targetDiskType, snapshotName, labels, 0, 0, storagePoolUrl)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockDiskClient.CreateDiskCalled)
	})

}

func TestUpdateDiskLabel(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	diskName := "test-disk"
	labelKey := "status"
	labelValue := "updated"

	t.Run("Success - Add new label", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			UpdateLabelErr: nil,
		}

		err := mockDiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockDiskClient.UpdateLabelCalled)
	})

	t.Run("Success - Update existing label", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			UpdateLabelErr: nil,
		}

		err := mockDiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockDiskClient.UpdateLabelCalled)
	})

	t.Run("Update Label Error", func(t *testing.T) {
		expectedErr := errors.New("update label failed")
		mockDiskClient := &mockDiskClient{
			UpdateLabelErr: expectedErr,
		}

		err := mockDiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockDiskClient.UpdateLabelCalled)
	})

	t.Run("Update Label with Custom Function", func(t *testing.T) {
		customErr := errors.New("custom update error")
		mockDiskClient := &mockDiskClient{
			UpdateDiskLabelFunc: func(ctx context.Context, projectID string, zone string, diskName string, labelKey string, labelValue string) error {
				return customErr
			},
		}

		err := mockDiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Equal(t, customErr, err)
		assert.True(t, mockDiskClient.UpdateLabelCalled)
	})
}

func TestDeleteDisk(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	diskName := "disk-to-delete"

	t.Run("Success", func(t *testing.T) {
		mockDiskClient := &mockDiskClient{
			DeleteDiskErr: nil,
		}

		err := mockDiskClient.DeleteDisk(ctx, projectID, zone, diskName)

		assert.NoError(t, err)
		assert.True(t, mockDiskClient.DeleteDiskCalled)
	})

	t.Run("Delete Disk API Error", func(t *testing.T) {
		expectedErr := errors.New("delete failed")
		mockDiskClient := &mockDiskClient{
			DeleteDiskErr: expectedErr,
		}

		err := mockDiskClient.DeleteDisk(ctx, projectID, zone, diskName)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.True(t, mockDiskClient.DeleteDiskCalled)
	})
}
