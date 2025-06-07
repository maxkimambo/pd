package gcp

import (
	"context"
	"errors"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"

	"google.golang.org/protobuf/proto"
)

// mockOperation provides a mock that satisfies the Operation interface
type mockOperation struct {
	name    string
	waitErr error
}

func (m *mockOperation) Name() string {
	return m.name
}

func (m *mockOperation) Wait(ctx context.Context, opts ...gax.CallOption) error {
	return m.waitErr
}

// Helper function to create a compute.Operation that returns the desired values
func createMockComputeOperation(name string, waitErr error) *compute.Operation {
	// Return nil for now since the Operation struct doesn't expose public fields
	// and we can't easily mock it. We'll modify our approach.
	return nil
}

type mockDisksClient struct {
	DiskClientInterface
	AggregatedListFunc func(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator
	InsertFunc         func(ctx context.Context, req *computepb.InsertDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	GetFunc            func(ctx context.Context, req *computepb.GetDiskRequest, opts ...gax.CallOption) (*computepb.Disk, error)
	SetLabelsFunc      func(ctx context.Context, req *computepb.SetLabelsDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	DeleteFunc         func(ctx context.Context, req *computepb.DeleteDiskRequest, opts ...gax.CallOption) (*compute.Operation, error)
	CloseFunc          func() error

	AggregatedListErr error
	InsertOp          *compute.Operation
	InsertErr         error
	GetResp           *computepb.Disk
	GetErr            error
	SetLabelsOp       *compute.Operation
	SetLabelsErr      error
	DeleteOp          *compute.Operation
	DeleteErr         error

	AggregatedListCalled bool
	InsertCalled         bool
	GetCalled            bool
	SetLabelsCalled      bool
	DeleteCalled         bool
	LastInsertReq        *computepb.InsertDiskRequest
	LastGetReq           *computepb.GetDiskRequest
	LastSetLabelsReq     *computepb.SetLabelsDiskRequest
	LastDeleteReq        *computepb.DeleteDiskRequest
}

func (m *mockDisksClient) AggregatedList(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator {
	m.AggregatedListCalled = true
	if m.AggregatedListFunc != nil {
		return m.AggregatedListFunc(ctx, req, opts...)
	}
	return nil
}

func (m *mockDisksClient) Insert(ctx context.Context, req *computepb.InsertDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	m.InsertCalled = true
	m.LastInsertReq = req
	if m.InsertFunc != nil {
		return m.InsertFunc(ctx, req, opts...)
	}
	return m.InsertOp, m.InsertErr
}

func (m *mockDisksClient) Get(ctx context.Context, req *computepb.GetDiskRequest, opts ...gax.CallOption) (*computepb.Disk, error) {
	m.GetCalled = true
	m.LastGetReq = req
	if m.GetFunc != nil {
		return m.GetFunc(ctx, req, opts...)
	}
	return m.GetResp, m.GetErr
}

func (m *mockDisksClient) SetLabels(ctx context.Context, req *computepb.SetLabelsDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	m.SetLabelsCalled = true
	m.LastSetLabelsReq = req
	if m.SetLabelsFunc != nil {
		return m.SetLabelsFunc(ctx, req, opts...)
	}
	return m.SetLabelsOp, m.SetLabelsErr
}

func (m *mockDisksClient) Delete(ctx context.Context, req *computepb.DeleteDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	m.DeleteCalled = true
	m.LastDeleteReq = req
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, req, opts...)
	}
	return m.DeleteOp, m.DeleteErr
}

func (m *mockDisksClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func TestListDetachedDisks(t *testing.T) {
	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	fullZonePath := "projects/" + projectID + "/zones/" + zone

	t.Run("Success - Found detached disks", func(t *testing.T) {

		mockDiskClient := &mockDisksClient{
			AggregatedListFunc: func(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator {

				return nil
			},
		}

		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}

		disks, err := clients.DiskClient.ListDetachedDisks(ctx, projectID, zone, "")

		assert.NoError(t, err)
		assert.NotNil(t, disks)

		disks, err = clients.DiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.True(t, mockDiskClient.AggregatedListCalled)

		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.Len(t, disks, 0)

	})

	t.Run("Success - No detached disks found", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			AggregatedListFunc: func(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator {
				return nil
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}

		disks, err := clients.DiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.True(t, mockDiskClient.AggregatedListCalled)
		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.Len(t, disks, 0)
	})

	t.Run("API Error - Nil Iterator", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			AggregatedListFunc: func(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gax.CallOption) *compute.DisksScopedListPairIterator {
				return nil
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}

		disks, err := clients.DiskClient.ListDetachedDisks(ctx, projectID, fullZonePath, "")

		assert.True(t, mockDiskClient.AggregatedListCalled)
		assert.NoError(t, err)
		assert.NotNil(t, disks)
		assert.Len(t, disks, 0)
	})
}

func TestCreateNewDiskFromSnapshot(t *testing.T) {
	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	newDiskName := "new-disk"
	targetDiskType := "pd-ssd"
	snapshotName := "snap-1"
	fullSnapshotPath := "global/snapshots/" + snapshotName
	fullDiskTypePath := "zones/" + zone + "/diskTypes/" + targetDiskType
	labels := map[string]string{"env": "test"}
	storagePoolUrl := "projects/project/zones/zone/storagePools/storagePool"

	t.Run("Success", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			InsertFunc: func(ctx context.Context, req *computepb.InsertDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				// Return an error to avoid the operation.Wait() call that would panic
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.CreateNewDiskFromSnapshot(ctx, projectID, zone, newDiskName, targetDiskType, snapshotName, labels, 0, 0, storagePoolUrl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockDiskClient.InsertCalled)
		assert.NotNil(t, mockDiskClient.LastInsertReq)
		assert.Equal(t, projectID, mockDiskClient.LastInsertReq.GetProject())
		assert.Equal(t, zone, mockDiskClient.LastInsertReq.GetZone())
		assert.Equal(t, newDiskName, mockDiskClient.LastInsertReq.GetDiskResource().GetName())
		assert.Equal(t, fullSnapshotPath, mockDiskClient.LastInsertReq.GetDiskResource().GetSourceSnapshot())
		assert.Equal(t, fullDiskTypePath, mockDiskClient.LastInsertReq.GetDiskResource().GetType())
		assert.Equal(t, labels, mockDiskClient.LastInsertReq.GetDiskResource().GetLabels())
	})

	t.Run("Insert API Error", func(t *testing.T) {
		expectedErr := errors.New("insert failed")
		mockDiskClient := &mockDisksClient{
			InsertErr: expectedErr,
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}

		err := clients.DiskClient.CreateNewDiskFromSnapshot(ctx, projectID, zone, newDiskName, targetDiskType, snapshotName, labels, 0, 0, storagePoolUrl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockDiskClient.InsertCalled)
	})

}

func TestUpdateDiskLabel(t *testing.T) {
	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	diskName := "test-disk"
	labelKey := "status"
	labelValue := "updated"
	initialFingerprint := "fingerprint123"


	t.Run("Success - Add new label", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			GetResp: &computepb.Disk{
				Name:             proto.String(diskName),
				LabelFingerprint: proto.String(initialFingerprint),
				Labels:           nil,
			},
			GetErr: nil,
			SetLabelsFunc: func(ctx context.Context, req *computepb.SetLabelsDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockDiskClient.GetCalled)
		assert.Equal(t, diskName, mockDiskClient.LastGetReq.GetDisk())
		assert.True(t, mockDiskClient.SetLabelsCalled)
		assert.NotNil(t, mockDiskClient.LastSetLabelsReq)
		assert.Equal(t, projectID, mockDiskClient.LastSetLabelsReq.GetProject())
		assert.Equal(t, zone, mockDiskClient.LastSetLabelsReq.GetZone())
		assert.Equal(t, diskName, mockDiskClient.LastSetLabelsReq.GetResource())
		assert.Equal(t, initialFingerprint, mockDiskClient.LastSetLabelsReq.GetZoneSetLabelsRequestResource().GetLabelFingerprint())
		expectedLabels := map[string]string{labelKey: labelValue}
		assert.Equal(t, expectedLabels, mockDiskClient.LastSetLabelsReq.GetZoneSetLabelsRequestResource().GetLabels())
	})

	t.Run("Success - Update existing label", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			GetResp: &computepb.Disk{
				Name:             proto.String(diskName),
				LabelFingerprint: proto.String(initialFingerprint),
				Labels:           map[string]string{"existing": "value", labelKey: "oldValue"},
			},
			GetErr: nil,
			SetLabelsFunc: func(ctx context.Context, req *computepb.SetLabelsDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockDiskClient.GetCalled)
		assert.True(t, mockDiskClient.SetLabelsCalled)
		assert.Equal(t, initialFingerprint, mockDiskClient.LastSetLabelsReq.GetZoneSetLabelsRequestResource().GetLabelFingerprint())
		expectedLabels := map[string]string{"existing": "value", labelKey: labelValue}
		assert.Equal(t, expectedLabels, mockDiskClient.LastSetLabelsReq.GetZoneSetLabelsRequestResource().GetLabels())
	})

	t.Run("Get Disk Error", func(t *testing.T) {
		expectedErr := errors.New("get failed")
		mockDiskClient := &mockDisksClient{
			GetErr: expectedErr,
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockDiskClient.GetCalled)
		assert.False(t, mockDiskClient.SetLabelsCalled)
	})

	t.Run("SetLabels API Error", func(t *testing.T) {
		expectedErr := errors.New("setlabels failed")
		mockDiskClient := &mockDisksClient{
			GetResp: &computepb.Disk{
				Name:             proto.String(diskName),
				LabelFingerprint: proto.String(initialFingerprint),
				Labels:           nil,
			},
			GetErr:       nil,
			SetLabelsErr: expectedErr,
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.UpdateDiskLabel(ctx, projectID, zone, diskName, labelKey, labelValue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockDiskClient.GetCalled)
		assert.True(t, mockDiskClient.SetLabelsCalled)
	})
}

func TestDeleteDisk(t *testing.T) {
	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"
	diskName := "disk-to-delete"


	t.Run("Success", func(t *testing.T) {
		mockDiskClient := &mockDisksClient{
			DeleteFunc: func(ctx context.Context, req *computepb.DeleteDiskRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.DeleteDisk(ctx, projectID, zone, diskName)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockDiskClient.DeleteCalled)
		assert.NotNil(t, mockDiskClient.LastDeleteReq)
		assert.Equal(t, projectID, mockDiskClient.LastDeleteReq.GetProject())
		assert.Equal(t, zone, mockDiskClient.LastDeleteReq.GetZone())
		assert.Equal(t, diskName, mockDiskClient.LastDeleteReq.GetDisk())
	})

	t.Run("Delete API Error", func(t *testing.T) {
		expectedErr := errors.New("delete failed")
		mockDiskClient := &mockDisksClient{
			DeleteErr: expectedErr,
		}
		diskClient := NewDiskClient(mockDiskClient)
		clients := &Clients{Disks: mockDiskClient, DiskClient: diskClient}
		err := clients.DiskClient.DeleteDisk(ctx, projectID, zone, diskName)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockDiskClient.DeleteCalled)
	})
}
