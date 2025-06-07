package gcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
)


type mockSnapshotsClient struct {
	SnapshotClientInterface 
	InsertFunc func(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error)
	DeleteFunc func(ctx context.Context, req *computepb.DeleteSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error)
	ListFunc   func(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator
	CloseFunc  func() error

	InsertOp  *compute.Operation
	InsertErr error
	DeleteOp  *compute.Operation
	DeleteErr error

	InsertCalled  bool
	DeleteCalled  bool
	ListCalled    bool
	LastInsertReq *computepb.InsertSnapshotRequest
	LastDeleteReq *computepb.DeleteSnapshotRequest
	LastListReq   *computepb.ListSnapshotsRequest
}

func (m *mockSnapshotsClient) Insert(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	m.InsertCalled = true
	m.LastInsertReq = req
	if m.InsertFunc != nil {
		return m.InsertFunc(ctx, req, opts...)
	}
	return m.InsertOp, m.InsertErr
}

func (m *mockSnapshotsClient) Delete(ctx context.Context, req *computepb.DeleteSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	m.DeleteCalled = true
	m.LastDeleteReq = req
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, req, opts...)
	}
	return m.DeleteOp, m.DeleteErr
}

func (m *mockSnapshotsClient) List(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator {
	m.ListCalled = true
	m.LastListReq = req
	if m.ListFunc != nil {
		return m.ListFunc(ctx, req, opts...)
	}
	return nil
}

func (m *mockSnapshotsClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil 
}



func TestCreateSnapshot(t *testing.T) {
	ctx := context.Background()
	projectID := "test-proj"
	zone := "us-west1-a"
	diskName := "source-disk"
	snapshotName := "new-snap"
	sourceDiskURL := fmt.Sprintf("projects/%s/zones/%s/disks/%s", projectID, zone, diskName)

	t.Run("Success - No KMS, No extra labels", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			InsertFunc: func(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls") 
		assert.True(t, mockClient.InsertCalled)
		assert.NotNil(t, mockClient.LastInsertReq)
		assert.Equal(t, projectID, mockClient.LastInsertReq.GetProject())
		snapResource := mockClient.LastInsertReq.GetSnapshotResource()
		assert.NotNil(t, snapResource)
		assert.Equal(t, snapshotName, snapResource.GetName())
		assert.Equal(t, sourceDiskURL, snapResource.GetSourceDisk())
		expectedLabels := map[string]string{"managed-by": "pd-migrate"}
		assert.Equal(t, expectedLabels, snapResource.GetLabels())
		assert.Nil(t, snapResource.GetSnapshotEncryptionKey()) 
	})

	t.Run("Success - With KMS, With extra labels", func(t *testing.T) {
		kmsParams := &SnapshotKmsParams{
			KmsKey:      "my-key",
			KmsKeyRing:  "my-ring",
			KmsLocation: "global",
			KmsProject:  "kms-proj", 
		}
		initialLabels := map[string]string{"user": "test"}
		expectedKmsKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
			kmsParams.KmsProject, kmsParams.KmsLocation, kmsParams.KmsKeyRing, kmsParams.KmsKey)

		mockClient := &mockSnapshotsClient{
			InsertFunc: func(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, kmsParams, initialLabels)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockClient.InsertCalled)
		snapResource := mockClient.LastInsertReq.GetSnapshotResource()
		assert.NotNil(t, snapResource)
		expectedLabels := map[string]string{"managed-by": "pd-migrate", "user": "test"}
		assert.Equal(t, expectedLabels, snapResource.GetLabels())
		kmsKey := snapResource.GetSnapshotEncryptionKey()
		assert.NotNil(t, kmsKey)
		assert.Equal(t, expectedKmsKeyName, kmsKey.GetKmsKeyName())
	})

	t.Run("Success - With KMS, Default KMS project", func(t *testing.T) {
		kmsParams := &SnapshotKmsParams{ 
			KmsKey:      "my-key",
			KmsKeyRing:  "my-ring",
			KmsLocation: "global",
		}
		expectedKmsKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
			projectID, kmsParams.KmsLocation, kmsParams.KmsKeyRing, kmsParams.KmsKey) 

		mockClient := &mockSnapshotsClient{
			InsertFunc: func(ctx context.Context, req *computepb.InsertSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, kmsParams, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls")
		assert.True(t, mockClient.InsertCalled)
		snapResource := mockClient.LastInsertReq.GetSnapshotResource()
		kmsKey := snapResource.GetSnapshotEncryptionKey()
		assert.NotNil(t, kmsKey)
		assert.Equal(t, expectedKmsKeyName, kmsKey.GetKmsKeyName())
	})

	t.Run("Insert API Error", func(t *testing.T) {
		expectedErr := errors.New("snapshot insert failed")
		mockClient := &mockSnapshotsClient{
			InsertErr: expectedErr,
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.CreateSnapshot(ctx, projectID, zone, diskName, snapshotName, nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockClient.InsertCalled)
	})

}

func TestDeleteSnapshot(t *testing.T) {
	ctx := context.Background()
	projectID := "test-proj"
	snapshotName := "snap-to-delete"
	t.Run("Success", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			DeleteFunc: func(ctx context.Context, req *computepb.DeleteSnapshotRequest, opts ...gax.CallOption) (*compute.Operation, error) {
				return nil, errors.New("mocked to avoid operation calls")
			},
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.DeleteSnapshot(ctx, projectID, snapshotName)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mocked to avoid operation calls") 
		assert.True(t, mockClient.DeleteCalled)
		assert.NotNil(t, mockClient.LastDeleteReq)
		assert.Equal(t, projectID, mockClient.LastDeleteReq.GetProject())
		assert.Equal(t, snapshotName, mockClient.LastDeleteReq.GetSnapshot())
	})

	t.Run("Delete API Error", func(t *testing.T) {
		expectedErr := errors.New("snapshot delete failed")
		mockClient := &mockSnapshotsClient{
			DeleteErr: expectedErr,
		}
		clients := &Clients{Snapshots: mockClient}
		err := clients.DeleteSnapshot(ctx, projectID, snapshotName)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error())
		assert.True(t, mockClient.DeleteCalled)
	})

}

func TestListSnapshotsByLabel(t *testing.T) {
	ctx := context.Background()
	projectID := "test-proj"
	labelKey := "env"
	labelValue := "prod"
	expectedFilter := fmt.Sprintf("labels.%s = %s", labelKey, labelValue)

	t.Run("Success - Found snapshots", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			ListFunc: func(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator {

				return nil 
			},
		}
		clients := &Clients{Snapshots: mockClient}
		snapshots, err := clients.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.True(t, mockClient.ListCalled) 
		assert.NotNil(t, mockClient.LastListReq)
		assert.Equal(t, projectID, mockClient.LastListReq.GetProject())
		assert.Equal(t, expectedFilter, mockClient.LastListReq.GetFilter())

		assert.NoError(t, err)
		assert.NotNil(t, snapshots)
		assert.Len(t, snapshots, 0) 
	})

	t.Run("Success - No snapshots found", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			ListFunc: func(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator {
				return nil
			},
		}
		clients := &Clients{Snapshots: mockClient}
		snapshots, err := clients.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockClient.ListCalled)
		assert.Equal(t, expectedFilter, mockClient.LastListReq.GetFilter())
		assert.NotNil(t, snapshots)
		assert.Len(t, snapshots, 0)
	})

	t.Run("List API Error", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			ListFunc: func(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator {
				return nil
			},
		}
		clients := &Clients{Snapshots: mockClient}
		snapshots, err := clients.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err) 
		assert.True(t, mockClient.ListCalled)
		assert.NotNil(t, snapshots) 
		assert.Len(t, snapshots, 0)
	})

	t.Run("Iterator Error during iteration", func(t *testing.T) {
		mockClient := &mockSnapshotsClient{
			ListFunc: func(ctx context.Context, req *computepb.ListSnapshotsRequest, opts ...gax.CallOption) *compute.SnapshotIterator {
				return nil
			},
		}
		clients := &Clients{Snapshots: mockClient}
		snapshots, err := clients.ListSnapshotsByLabel(ctx, projectID, labelKey, labelValue)

		assert.NoError(t, err)
		assert.True(t, mockClient.ListCalled)
		assert.NotNil(t, snapshots)
		assert.Len(t, snapshots, 0)
	})
}
