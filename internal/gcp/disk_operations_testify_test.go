package gcp

import (
	"context"
	"errors"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp/mocks"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

// TestWithTestifyMock demonstrates the standardized testify/mock approach
func TestListDetachedDisks_WithTestifyMock(t *testing.T) {
	// Initialize logger (required for operations)
	logger.Setup(false, false, false)

	tests := []struct {
		name          string
		projectID     string
		location      string
		labelFilter   string
		mockResp      []*computepb.Disk
		mockErr       error
		expectedDisks int
		expectedError bool
	}{
		{
			name:        "Success - Found detached disks",
			projectID:   "test-project",
			location:    "us-west1-a",
			labelFilter: "environment=test",
			mockResp: []*computepb.Disk{
				{
					Name: proto.String("test-disk-1"),
					Zone: proto.String("projects/test-project/zones/us-west1-a"),
				},
				{
					Name: proto.String("test-disk-2"),
					Zone: proto.String("projects/test-project/zones/us-west1-a"),
				},
			},
			mockErr:       nil,
			expectedDisks: 2,
			expectedError: false,
		},
		{
			name:          "Success - No detached disks found",
			projectID:     "test-project",
			location:      "us-west1-a",
			labelFilter:   "environment=nonexistent",
			mockResp:      []*computepb.Disk{},
			mockErr:       nil,
			expectedDisks: 0,
			expectedError: false,
		},
		{
			name:          "API Error - List operation fails",
			projectID:     "test-project",
			location:      "us-west1-a",
			labelFilter:   "environment=test",
			mockResp:      nil,
			mockErr:       errors.New("API error: failed to list disks"),
			expectedDisks: 0,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock using testify/mock
			mockClient := mocks.NewMockDiskClientInterface(t)

			// Set up expectations using the fluent API
			mockClient.EXPECT().
				ListDetachedDisks(mock.Anything, tt.projectID, tt.location, tt.labelFilter).
				Return(tt.mockResp, tt.mockErr).
				Once()

			// Execute the test
			disks, err := mockClient.ListDetachedDisks(context.Background(), tt.projectID, tt.location, tt.labelFilter)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, disks)
			} else {
				assert.NoError(t, err)
				assert.Len(t, disks, tt.expectedDisks)

				// Verify disk details if any
				if tt.expectedDisks > 0 {
					for i, disk := range disks {
						assert.Equal(t, tt.mockResp[i].GetName(), disk.GetName())
						assert.Equal(t, tt.mockResp[i].GetZone(), disk.GetZone())
					}
				}
			}

			// Mock expectations are automatically verified by testify
			// via the cleanup function registered in mocks.NewMockDiskClientInterface
		})
	}
}

func TestGetDisk_WithTestifyMock(t *testing.T) {
	// Initialize logger
	logger.Setup(false, false, false)

	tests := []struct {
		name         string
		projectID    string
		zone         string
		diskName     string
		mockResponse *computepb.Disk
		mockError    error
		expectError  bool
	}{
		{
			name:      "Success",
			projectID: "test-project",
			zone:      "us-west1-a",
			diskName:  "test-disk",
			mockResponse: &computepb.Disk{
				Name:   proto.String("test-disk"),
				Zone:   proto.String("projects/test-project/zones/us-west1-a"),
				SizeGb: proto.Int64(100),
				Status: proto.String("READY"),
				Type:   proto.String("projects/test-project/zones/us-west1-a/diskTypes/pd-standard"),
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "Get Disk Error",
			projectID:    "test-project",
			zone:         "us-west1-a",
			diskName:     "nonexistent-disk",
			mockResponse: nil,
			mockError:    errors.New("disk not found"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock using testify/mock
			mockClient := mocks.NewMockDiskClientInterface(t)

			// Set up expectations
			mockClient.EXPECT().
				GetDisk(mock.Anything, tt.projectID, tt.zone, tt.diskName).
				Return(tt.mockResponse, tt.mockError).
				Once()

			// Execute
			disk, err := mockClient.GetDisk(context.Background(), tt.projectID, tt.zone, tt.diskName)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, disk)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, disk)
				assert.Equal(t, tt.mockResponse.GetName(), disk.GetName())
				assert.Equal(t, tt.mockResponse.GetZone(), disk.GetZone())
				assert.Equal(t, tt.mockResponse.GetSizeGb(), disk.GetSizeGb())
				assert.Equal(t, tt.mockResponse.GetStatus(), disk.GetStatus())
			}
		})
	}
}

func TestCreateNewDiskFromSnapshot_WithTestifyMock(t *testing.T) {
	// Initialize logger
	logger.Setup(false, false, false)

	tests := []struct {
		name           string
		projectID      string
		zone           string
		newDiskName    string
		targetDiskType string
		snapshotSource string
		labels         map[string]string
		size           int64
		iops           int64
		throughput     int64
		storagePoolID  string
		mockError      error
		expectError    bool
	}{
		{
			name:           "Success",
			projectID:      "test-project",
			zone:           "us-west1-a",
			newDiskName:    "new-disk",
			targetDiskType: "pd-ssd",
			snapshotSource: "projects/test-project/global/snapshots/test-snapshot",
			labels:         map[string]string{"environment": "test"},
			size:           100,
			iops:           3000,
			throughput:     140,
			storagePoolID:  "",
			mockError:      nil,
			expectError:    false,
		},
		{
			name:           "Create Disk API Error",
			projectID:      "test-project",
			zone:           "us-west1-a",
			newDiskName:    "new-disk",
			targetDiskType: "pd-ssd",
			snapshotSource: "projects/test-project/global/snapshots/test-snapshot",
			labels:         map[string]string{"environment": "test"},
			size:           100,
			iops:           3000,
			throughput:     140,
			storagePoolID:  "",
			mockError:      errors.New("failed to create disk"),
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock
			mockClient := mocks.NewMockDiskClientInterface(t)

			// Set up expectations with argument matching
			mockClient.EXPECT().
				CreateNewDiskFromSnapshot(
					mock.Anything, // context
					tt.projectID,
					tt.zone,
					tt.newDiskName,
					tt.targetDiskType,
					tt.snapshotSource,
					tt.labels,
					tt.size,
					tt.iops,
					tt.throughput,
					tt.storagePoolID,
				).
				Return(tt.mockError).
				Once()

			// Execute
			err := mockClient.CreateNewDiskFromSnapshot(
				context.Background(),
				tt.projectID,
				tt.zone,
				tt.newDiskName,
				tt.targetDiskType,
				tt.snapshotSource,
				tt.labels,
				tt.size,
				tt.iops,
				tt.throughput,
				tt.storagePoolID,
			)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Example of advanced testify/mock features
func TestAdvancedMockFeatures(t *testing.T) {
	mockClient := mocks.NewMockDiskClientInterface(t)

	t.Run("Custom argument matching", func(t *testing.T) {
		// Use custom matchers for more flexible argument matching
		mockClient.EXPECT().
			GetDisk(
				mock.Anything, // any context
				mock.MatchedBy(func(projectID string) bool {
					return projectID != ""
				}), // non-empty project ID
				"us-west1-a",
				mock.AnythingOfType("string"),
			).
			Return(&computepb.Disk{Name: proto.String("matched-disk")}, nil).
			Once()

		disk, err := mockClient.GetDisk(context.Background(), "any-project", "us-west1-a", "any-disk")
		assert.NoError(t, err)
		assert.Equal(t, "matched-disk", disk.GetName())
	})

	t.Run("Multiple call expectations", func(t *testing.T) {
		// Expect multiple calls with different returns
		mockClient.EXPECT().
			ListDetachedDisks(mock.Anything, "project1", mock.Anything, mock.Anything).
			Return([]*computepb.Disk{}, nil).
			Times(2)

		// Call twice
		_, err1 := mockClient.ListDetachedDisks(context.Background(), "project1", "zone1", "")
		_, err2 := mockClient.ListDetachedDisks(context.Background(), "project1", "zone2", "")

		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

	t.Run("Custom function execution", func(t *testing.T) {
		// Use Run to execute custom logic
		var capturedProjectID string
		mockClient.EXPECT().
			DeleteDisk(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(ctx context.Context, projectID, zone, diskName string) {
				capturedProjectID = projectID
			}).
			Return(nil).
			Once()

		err := mockClient.DeleteDisk(context.Background(), "captured-project", "zone", "disk")
		assert.NoError(t, err)
		assert.Equal(t, "captured-project", capturedProjectID)
	})
}
