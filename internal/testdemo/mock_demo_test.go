package testdemo

import (
	"context"
	"errors"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

// TestStandardizedTestifyMockApproach demonstrates the standardized testify/mock approach
func TestStandardizedTestifyMockApproach(t *testing.T) {
	// Note: Logger will be initialized automatically when used

	tests := []struct {
		name           string
		projectID      string
		location       string
		labelFilter    string
		mockResp       []*computepb.Disk
		mockErr        error
		expectedDisks  int
		expectedError  bool
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
			// Create mock using testify/mock - this demonstrates the standardized approach
			mockClient := mocks.NewMockDiskClientInterface(t)
			
			// Set up expectations using the fluent expectation API
			mockClient.EXPECT().
				ListDetachedDisks(mock.Anything, tt.projectID, tt.location, tt.labelFilter).
				Return(tt.mockResp, tt.mockErr).
				Once()

			// Execute the test - calling the mocked method
			disks, err := mockClient.ListDetachedDisks(context.Background(), tt.projectID, tt.location, tt.labelFilter)

			// Verify results using testify/assert
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
			// via the cleanup function registered in NewMockDiskClientInterface
		})
	}
}

// TestAdvancedMockFeatures demonstrates advanced testify/mock capabilities
func TestAdvancedMockFeatures(t *testing.T) {
	t.Run("Custom argument matching", func(t *testing.T) {
		mockClient := mocks.NewMockDiskClientInterface(t)
		
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
		mockClient := mocks.NewMockDiskClientInterface(t)
		
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
		mockClient := mocks.NewMockDiskClientInterface(t)
		
		// Use Run to execute custom logic during mock call
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

	t.Run("Conditional returns based on arguments", func(t *testing.T) {
		mockClient := mocks.NewMockDiskClientInterface(t)
		
		// Return different results based on input
		mockClient.EXPECT().
			GetDisk(mock.Anything, mock.Anything, mock.Anything, "found-disk").
			Return(&computepb.Disk{Name: proto.String("found-disk")}, nil).
			Once()

		mockClient.EXPECT().
			GetDisk(mock.Anything, mock.Anything, mock.Anything, "missing-disk").
			Return(nil, errors.New("disk not found")).
			Once()

		// Test both cases
		foundDisk, err1 := mockClient.GetDisk(context.Background(), "project", "zone", "found-disk")
		assert.NoError(t, err1)
		assert.Equal(t, "found-disk", foundDisk.GetName())

		missingDisk, err2 := mockClient.GetDisk(context.Background(), "project", "zone", "missing-disk")
		assert.Error(t, err2)
		assert.Nil(t, missingDisk)
	})
}