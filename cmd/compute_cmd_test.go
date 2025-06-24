package cmd

import (
	"context"
	"testing"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/gcp/mocks"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

func TestValidateInstancesForMigration(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)
	
	ctx := context.Background()
	projectID := "test-project"

	tests := []struct {
		name                 string
		instances            []*computepb.Instance
		targetDiskType       string
		expectedValidated    int
		expectedFailed       int
		expectedFailureNames []string
		mockDiskResponses    map[string]*computepb.Disk
		mockDiskErrors       map[string]error
	}{
		{
			name: "incompatible machine type - f1-micro with hyperdisk-balanced",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("test-instance-1"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/f1-micro"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/test-disk-1"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    0,
			expectedFailed:       1,
			expectedFailureNames: []string{"test-instance-1"},
			mockDiskResponses: map[string]*computepb.Disk{
				// No disk responses needed since machine type validation fails first
			},
		},
		{
			name: "compatible machine type - c3-standard-4 with hyperdisk-balanced",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("test-instance-2"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/test-disk-2"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    1,
			expectedFailed:       0,
			expectedFailureNames: []string{},
			mockDiskResponses: map[string]*computepb.Disk{
				"test-disk-2": {
					Name: stringPtr("test-disk-2"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/pd-standard"),
				},
			},
		},
		{
			name: "mixed compatibility - one compatible, one incompatible",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("compatible-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/compatible-disk"),
						},
					},
				},
				{
					Name: stringPtr("incompatible-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/f1-micro"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/incompatible-disk"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    1,
			expectedFailed:       1,
			expectedFailureNames: []string{"incompatible-instance"},
			mockDiskResponses: map[string]*computepb.Disk{
				"compatible-disk": {
					Name: stringPtr("compatible-disk"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/pd-standard"),
				},
				// No disk response needed for incompatible-disk since machine type fails first
			},
		},
		{
			name: "instance with local-ssd disk - should fail disk validation",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("local-ssd-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/local-ssd-disk"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    0,
			expectedFailed:       1,
			expectedFailureNames: []string{"local-ssd-instance"},
			mockDiskResponses: map[string]*computepb.Disk{
				"local-ssd-disk": {
					Name: stringPtr("local-ssd-disk"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/local-ssd"),
				},
			},
		},
		{
			name: "instance with disk already target type - should pass",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("already-target-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/already-target-disk"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    1,
			expectedFailed:       0,
			expectedFailureNames: []string{},
			mockDiskResponses: map[string]*computepb.Disk{
				"already-target-disk": {
					Name: stringPtr("already-target-disk"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/hyperdisk-balanced"),
				},
			},
		},
		{
			name: "instance with no disks - should pass",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("no-disks-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    1,
			expectedFailed:       0,
			expectedFailureNames: []string{},
			mockDiskResponses:    map[string]*computepb.Disk{},
		},
		{
			name: "instance with multiple disks - some local-ssd",
			instances: []*computepb.Instance{
				{
					Name: stringPtr("multi-disk-instance"),
					MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
					Zone: stringPtr("projects/test-project/zones/us-central1-a"),
					Disks: []*computepb.AttachedDisk{
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/good-disk"),
						},
						{
							Source: stringPtr("projects/test-project/zones/us-central1-a/disks/bad-disk"),
						},
					},
				},
			},
			targetDiskType:       "hyperdisk-balanced",
			expectedValidated:    0,
			expectedFailed:       1,
			expectedFailureNames: []string{"multi-disk-instance"},
			mockDiskResponses: map[string]*computepb.Disk{
				"good-disk": {
					Name: stringPtr("good-disk"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/pd-standard"),
				},
				"bad-disk": {
					Name: stringPtr("bad-disk"),
					Type: stringPtr("projects/test-project/zones/us-central1-a/diskTypes/local-ssd"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock GCP clients
			mockDiskClient := &mocks.MockDiskClientInterface{}
			gcpClient := &gcp.Clients{
				DiskClient: mockDiskClient,
			}

			// Setup mock expectations for disk client
			for diskName, diskResponse := range tt.mockDiskResponses {
				if diskResponse != nil {
					mockDiskClient.On("GetDisk", mock.Anything, projectID, "us-central1-a", diskName).Return(diskResponse, nil)
				}
			}

			for diskName, diskError := range tt.mockDiskErrors {
				mockDiskClient.On("GetDisk", mock.Anything, projectID, "us-central1-a", diskName).Return(nil, diskError)
			}

			// Execute the validation function
			validatedInstances, failedInstances, err := validateInstancesForMigration(ctx, tt.instances, tt.targetDiskType, projectID, gcpClient)

			// Assertions
			assert.NoError(t, err, "validateInstancesForMigration should not return an error")
			assert.Len(t, validatedInstances, tt.expectedValidated, "Number of validated instances should match expected")
			assert.Len(t, failedInstances, tt.expectedFailed, "Number of failed instances should match expected")

			// Check that the expected instances failed
			var actualFailureNames []string
			for _, failure := range failedInstances {
				actualFailureNames = append(actualFailureNames, failure.InstanceName)
			}
			assert.ElementsMatch(t, tt.expectedFailureNames, actualFailureNames, "Failed instance names should match expected")

			// Verify all expectations were met
			mockDiskClient.AssertExpectations(t)
		})
	}
}

func TestValidateInstancesForMigration_EdgeCases(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)
	
	ctx := context.Background()
	projectID := "test-project"

	t.Run("empty instance list", func(t *testing.T) {
		mockDiskClient := &mocks.MockDiskClientInterface{}
		gcpClient := &gcp.Clients{
			DiskClient: mockDiskClient,
		}

		validatedInstances, failedInstances, err := validateInstancesForMigration(ctx, []*computepb.Instance{}, "hyperdisk-balanced", projectID, gcpClient)

		assert.NoError(t, err)
		assert.Empty(t, validatedInstances)
		assert.Empty(t, failedInstances)
	})

	t.Run("instance with disk without source", func(t *testing.T) {
		instances := []*computepb.Instance{
			{
				Name: stringPtr("instance-with-no-source-disk"),
				MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/c3-standard-4"),
				Zone: stringPtr("projects/test-project/zones/us-central1-a"),
				Disks: []*computepb.AttachedDisk{
					{
						Source: stringPtr(""), // Empty source
					},
				},
			},
		}

		mockDiskClient := &mocks.MockDiskClientInterface{}
		gcpClient := &gcp.Clients{
			DiskClient: mockDiskClient,
		}

		validatedInstances, failedInstances, err := validateInstancesForMigration(ctx, instances, "hyperdisk-balanced", projectID, gcpClient)

		assert.NoError(t, err)
		assert.Len(t, validatedInstances, 1) // Should pass since no disks to validate
		assert.Empty(t, failedInstances)
	})

	t.Run("unknown machine type", func(t *testing.T) {
		instances := []*computepb.Instance{
			{
				Name: stringPtr("unknown-machine-type-instance"),
				MachineType: stringPtr("projects/test-project/zones/us-central1-a/machineTypes/unknown-machine-type"),
				Zone: stringPtr("projects/test-project/zones/us-central1-a"),
				Disks: []*computepb.AttachedDisk{
					{
						Source: stringPtr("projects/test-project/zones/us-central1-a/disks/test-disk"),
					},
				},
			},
		}

		mockDiskClient := &mocks.MockDiskClientInterface{}
		// No disk mock needed since machine type validation fails first

		gcpClient := &gcp.Clients{
			DiskClient: mockDiskClient,
		}

		validatedInstances, failedInstances, err := validateInstancesForMigration(ctx, instances, "hyperdisk-balanced", projectID, gcpClient)

		assert.NoError(t, err)
		assert.Empty(t, validatedInstances)
		assert.Len(t, failedInstances, 1)
		assert.Contains(t, failedInstances[0].Reason, "unknown-machine-type does not support disk type hyperdisk-balanced")
		
		mockDiskClient.AssertExpectations(t)
	})
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}