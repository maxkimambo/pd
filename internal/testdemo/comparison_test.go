package testdemo

import (
	"context"
	"errors"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/gcp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

// ManualMockApproach demonstrates the old manual mock approach
func TestManualMockApproach(t *testing.T) {
	// Manual mock implementation (old approach)
	type manualMockDiskClient struct {
		gcp.DiskClientInterface
		GetDiskResp *computepb.Disk
		GetDiskErr  error
		GetDiskCalled bool
		// Plus all the other fields and methods needed...
	}
	
	// Manual implementation of interface method
	getMockDiskClient := func() *manualMockDiskClient {
		return &manualMockDiskClient{}
	}
	
	mockClient := getMockDiskClient()
	mockClient.GetDiskResp = &computepb.Disk{Name: proto.String("manual-disk")}
	mockClient.GetDiskErr = nil
	
	// We would need to implement the actual GetDisk method manually
	// This is a lot of boilerplate code...
	
	// For demo purposes, we'll just verify the setup
	assert.NotNil(t, mockClient)
	assert.Equal(t, "manual-disk", mockClient.GetDiskResp.GetName())
}

// TestifyMockApproach demonstrates the new standardized approach
func TestTestifyMockApproach(t *testing.T) {
	// Generated mock (new approach) - much cleaner!
	mockClient := mocks.NewMockDiskClientInterface(t)
	
	// Set up expectations using fluent API
	mockClient.EXPECT().
		GetDisk(mock.Anything, "project", "zone", "disk").
		Return(&computepb.Disk{Name: proto.String("testify-disk")}, nil).
		Once()
	
	// Execute the test
	disk, err := mockClient.GetDisk(context.Background(), "project", "zone", "disk")
	
	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, "testify-disk", disk.GetName())
	
	// No need to manually verify mock calls - testify does it automatically!
}

// SideBySideComparison shows the difference in complexity
func TestSideBySideComparison(t *testing.T) {
	t.Run("Old Manual Approach Problems", func(t *testing.T) {
		// Problems with manual mocks:
		// 1. Lots of boilerplate code for each interface
		// 2. Need to manually implement every interface method
		// 3. Manual call tracking and verification
		// 4. Inconsistent patterns across different test files
		// 5. Hard to maintain when interfaces change
		// 6. Limited expectation management
		
		// Example of manual mock complexity:
		type manualMock struct {
			gcp.DiskClientInterface
			// Fields for each method
			GetDiskResp *computepb.Disk
			GetDiskErr  error
			GetDiskCalled bool
			GetDiskCallCount int
			LastGetDiskProjectID string
			LastGetDiskZone string
			LastGetDiskName string
			
			// Plus similar fields for every other method...
			// ListDetachedDisksResp, CreateDiskResp, etc.
		}
		
		// And then manual implementation of every method...
		// This quickly becomes unwieldy!
		
		assert.True(t, true, "Manual approach requires lots of boilerplate")
	})
	
	t.Run("New Testify Approach Benefits", func(t *testing.T) {
		// Benefits of testify/mock:
		// 1. Zero boilerplate - mocks are generated
		// 2. Rich expectation API
		// 3. Automatic call verification
		// 4. Consistent patterns across all tests
		// 5. Easy maintenance when interfaces change
		// 6. Type safety guaranteed
		
		mockClient := mocks.NewMockDiskClientInterface(t)
		
		// Demonstrate rich expectation API
		mockClient.EXPECT().
			GetDisk(
				mock.Anything,  // flexible context matching
				mock.MatchedBy(func(projectID string) bool {
					return len(projectID) > 0  // custom validation
				}),
				"us-west1-a",
				mock.AnythingOfType("string"),
			).
			Return(&computepb.Disk{Name: proto.String("advanced-disk")}, nil).
			Once()
		
		// Execute
		disk, err := mockClient.GetDisk(context.Background(), "valid-project", "us-west1-a", "any-disk")
		
		// Verify
		assert.NoError(t, err)
		assert.Equal(t, "advanced-disk", disk.GetName())
		
		// All expectations are automatically verified!
	})
}

// ComplexScenarioComparison demonstrates handling complex testing scenarios
func TestComplexScenarioComparison(t *testing.T) {
	t.Run("Multiple Interface Dependencies", func(t *testing.T) {
		// With testify/mock, it's easy to coordinate multiple mocks
		diskMock := mocks.NewMockDiskClientInterface(t)
		// We could add snapshot mock here too if the interface was available
		
		// Set up coordinated expectations
		diskMock.EXPECT().
			GetDisk(mock.Anything, "project", "zone", "source-disk").
			Return(&computepb.Disk{
				Name:   proto.String("source-disk"),
				SizeGb: proto.Int64(100),
			}, nil).
			Once()
		
		diskMock.EXPECT().
			CreateNewDiskFromSnapshot(
				mock.Anything,
				"project",
				"zone", 
				"new-disk",
				"pd-ssd",
				"snapshot-123",
				mock.Anything, // labels
				int64(100),    // size
				int64(3000),   // iops
				int64(140),    // throughput
				"",            // storage pool
			).
			Return(nil).
			Once()
		
		// Simulate a migration operation
		sourceDisk, err := diskMock.GetDisk(context.Background(), "project", "zone", "source-disk")
		assert.NoError(t, err)
		
		err = diskMock.CreateNewDiskFromSnapshot(
			context.Background(),
			"project", "zone", "new-disk", "pd-ssd", "snapshot-123",
			map[string]string{"migrated": "true"},
			sourceDisk.GetSizeGb(), 3000, 140, "",
		)
		assert.NoError(t, err)
		
		// All expectations verified automatically!
	})
	
	t.Run("Error Handling Scenarios", func(t *testing.T) {
		mockClient := mocks.NewMockDiskClientInterface(t)
		
		// Easy to test different error scenarios
		mockClient.EXPECT().
			GetDisk(mock.Anything, "project", "zone", "missing-disk").
			Return(nil, errors.New("disk not found")).
			Once()
		
		mockClient.EXPECT().
			GetDisk(mock.Anything, "project", "zone", "permission-denied").
			Return(nil, errors.New("permission denied")).
			Once()
		
		// Test missing disk
		disk1, err1 := mockClient.GetDisk(context.Background(), "project", "zone", "missing-disk")
		assert.Error(t, err1)
		assert.Nil(t, disk1)
		assert.Contains(t, err1.Error(), "not found")
		
		// Test permission error
		disk2, err2 := mockClient.GetDisk(context.Background(), "project", "zone", "permission-denied")
		assert.Error(t, err2)
		assert.Nil(t, disk2)
		assert.Contains(t, err2.Error(), "permission denied")
	})
}