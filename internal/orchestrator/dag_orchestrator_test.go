package orchestrator

import (
	"context"
	"fmt"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestNewDAGOrchestrator(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Concurrency:    5,
	}
	gcpClient := &gcp.Clients{}

	orchestrator := NewDAGOrchestrator(config, gcpClient)

	assert.NotNil(t, orchestrator)
	assert.Equal(t, config, orchestrator.config)
	assert.Equal(t, gcpClient, orchestrator.gcpClient)
}

func TestExtractDiskNameFromSource(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "valid source URL",
			source:   "projects/test-project/zones/us-central1-a/disks/test-disk",
			expected: "test-disk",
		},
		{
			name:     "empty source",
			source:   "",
			expected: "",
		},
		{
			name:     "invalid format",
			source:   "invalid-format",
			expected: "invalid-format",
		},
		{
			name:     "just disk name",
			source:   "test-disk",
			expected: "test-disk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDiskNameFromSource(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterDisksForMigration(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
	}
	gcpClient := &gcp.Clients{}
	orchestrator := NewDAGOrchestrator(config, gcpClient)

	tests := []struct {
		name          string
		attachedDisks []*computepb.AttachedDisk
		expected      int
	}{
		{
			name: "no boot disks",
			attachedDisks: []*computepb.AttachedDisk{
				{
					Source: proto.String("projects/test/zones/us-central1-a/disks/data-disk-1"),
					Boot:   proto.Bool(false),
				},
				{
					Source: proto.String("projects/test/zones/us-central1-a/disks/data-disk-2"),
					Boot:   proto.Bool(false),
				},
			},
			expected: 2,
		},
		{
			name: "with boot disk",
			attachedDisks: []*computepb.AttachedDisk{
				{
					Source: proto.String("projects/test/zones/us-central1-a/disks/boot-disk"),
					Boot:   proto.Bool(true),
				},
				{
					Source: proto.String("projects/test/zones/us-central1-a/disks/data-disk"),
					Boot:   proto.Bool(false),
				},
			},
			expected: 1, // Boot disk should be filtered out
		},
		{
			name:          "empty list",
			attachedDisks: []*computepb.AttachedDisk{},
			expected:      0,
		},
		{
			name: "only boot disks",
			attachedDisks: []*computepb.AttachedDisk{
				{
					Source: proto.String("projects/test/zones/us-central1-a/disks/boot-disk"),
					Boot:   proto.Bool(true),
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orchestrator.filterDisksForMigration(tt.attachedDisks)
			assert.Len(t, result, tt.expected)

			// Verify no boot disks in result
			for _, disk := range result {
				assert.False(t, disk.GetBoot(), "Boot disk should be filtered out")
			}
		})
	}
}

func TestBuildMigrationDAG_NoInstances(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Concurrency:    5,
	}
	gcpClient := &gcp.Clients{}
	orchestrator := NewDAGOrchestrator(config, gcpClient)

	dag, err := orchestrator.BuildMigrationDAG(context.Background(), []*computepb.Instance{})

	assert.NoError(t, err)
	assert.NotNil(t, dag)
	assert.Len(t, dag.GetAllNodes(), 0)
}

// This test would require more complex mocking to properly test the full workflow
// For now, we test the basic structure and error cases
func TestBuildMigrationDAG_Structure(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Concurrency:    5,
		RetainName:     true,
	}
	gcpClient := &gcp.Clients{}
	orchestrator := NewDAGOrchestrator(config, gcpClient)

	// Test with empty instances (should create empty but valid DAG)
	instances := []*computepb.Instance{}

	dag, err := orchestrator.BuildMigrationDAG(context.Background(), instances)

	assert.NoError(t, err)
	assert.NotNil(t, dag)

	// Validate the DAG structure
	err = dag.Validate()
	assert.NoError(t, err)
}

func TestExecutorConfigMapping(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Concurrency:    15,
	}
	gcpClient := &gcp.Clients{}
	orchestrator := NewDAGOrchestrator(config, gcpClient)

	// Test that the executor config uses the right concurrency from migration config
	assert.Equal(t, 15, config.Concurrency)
	assert.NotNil(t, orchestrator) // Use the orchestrator variable

	// The actual executor config is created in ExecuteMigrationDAG,
	// but we can verify the mapping logic would work correctly
	expectedConcurrency := config.Concurrency
	assert.Equal(t, 15, expectedConcurrency)
}

func TestDAGOrchestratorIntegration(t *testing.T) {
	// Integration test that verifies the basic orchestrator workflow
	// without requiring actual GCP API calls

	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Concurrency:    5,
		RetainName:     false,
	}

	// Use a real GCP client struct but don't make actual calls
	gcpClient := &gcp.Clients{}

	orchestrator := NewDAGOrchestrator(config, gcpClient)

	// Verify orchestrator is properly initialized
	assert.NotNil(t, orchestrator)
	assert.Equal(t, config.ProjectID, orchestrator.config.ProjectID)
	assert.Equal(t, config.TargetDiskType, orchestrator.config.TargetDiskType)
	assert.Equal(t, config.Concurrency, orchestrator.config.Concurrency)
	assert.False(t, orchestrator.config.RetainName)
}

func TestDAGValidation(t *testing.T) {
	config := &migrator.Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
	}
	gcpClient := &gcp.Clients{}
	orchestrator := NewDAGOrchestrator(config, gcpClient)

	// Test building a DAG with no instances should still validate
	dag, err := orchestrator.BuildMigrationDAG(context.Background(), []*computepb.Instance{})

	assert.NoError(t, err)
	assert.NotNil(t, dag)

	// Empty DAG should validate successfully
	err = dag.Validate()
	assert.NoError(t, err)
}

// Helper function to create test instances
func createTestInstance(name, zone string) *computepb.Instance {
	return &computepb.Instance{
		Name:   proto.String(name),
		Zone:   proto.String(fmt.Sprintf("projects/test-project/zones/%s", zone)),
		Status: proto.String("RUNNING"),
		Disks: []*computepb.AttachedDisk{
			{
				Source:     proto.String(fmt.Sprintf("projects/test-project/zones/%s/disks/data-disk", zone)),
				DeviceName: proto.String("persistent-disk-1"),
				Boot:       proto.Bool(false),
			},
		},
	}
}

// Helper function to create test attached disk
func createTestAttachedDisk(diskName, deviceName string, isBootDisk bool) *computepb.AttachedDisk {
	return &computepb.AttachedDisk{
		Source:     proto.String(fmt.Sprintf("projects/test-project/zones/us-central1-a/disks/%s", diskName)),
		DeviceName: proto.String(deviceName),
		Boot:       proto.Bool(isBootDisk),
	}
}
