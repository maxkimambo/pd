package migrator

import (
	"context"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

// MockDAGOrchestratorSimple implements the DAGOrchestrator interface for testing
type MockDAGOrchestratorSimple struct {
	mock.Mock
}

func (m *MockDAGOrchestratorSimple) BuildMigrationDAG(ctx context.Context, instances []*computepb.Instance) (interface{}, error) {
	args := m.Called(ctx, instances)
	return args.Get(0), args.Error(1)
}

func (m *MockDAGOrchestratorSimple) ExecuteMigrationDAG(ctx context.Context, migrationDAG interface{}) (interface{}, error) {
	args := m.Called(ctx, migrationDAG)
	return args.Get(0), args.Error(1)
}

func TestNewComputeDiskMigratorSimple(t *testing.T) {
	config := &Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	orchestrator := &MockDAGOrchestratorSimple{}

	migrator := NewComputeDiskMigrator(config, gcpClient, orchestrator)

	assert.NotNil(t, migrator)
	assert.Equal(t, config, migrator.config)
	assert.Equal(t, gcpClient, migrator.gcpClient)
	assert.NotNil(t, migrator.orchestrator)
}

func TestComputeDiskMigrator_MigrateInstanceDisksSimple(t *testing.T) {
	config := &Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	orchestrator := &MockDAGOrchestratorSimple{}

	// Setup orchestrator mocks
	instances := []*computepb.Instance{
		{
			Name:   proto.String("test-instance"),
			Status: proto.String("TERMINATED"),
			Zone:   proto.String("projects/test-project/zones/us-central1-a"),
		},
	}

	orchestrator.On("BuildMigrationDAG", mock.Anything, instances).
		Return(map[string]interface{}{"dag": "mock"}, nil)
	orchestrator.On("ExecuteMigrationDAG", mock.Anything, mock.Anything).
		Return(map[string]interface{}{"success": true}, nil)

	migrator := NewComputeDiskMigrator(config, gcpClient, orchestrator)

	ctx := context.Background()
	result, err := migrator.MigrateInstanceDisks(ctx, instances)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	orchestrator.AssertExpectations(t)
}

func TestComputeDiskMigratorInterface(t *testing.T) {
	// Test that the ComputeDiskMigrator implements the expected interface
	config := &Config{
		ProjectID:      "test-project",
		TargetDiskType: "pd-ssd",
		Zone:           "us-central1-a",
	}
	gcpClient := &gcp.Clients{}
	orchestrator := &MockDAGOrchestratorSimple{}

	migrator := NewComputeDiskMigrator(config, gcpClient, orchestrator)

	// Test that the migrator has the expected methods
	assert.NotNil(t, migrator)

	// Test MigrateInstanceDisks method exists and returns correct types
	ctx := context.Background()
	instances := []*computepb.Instance{}

	// Setup mock expectations
	orchestrator.On("BuildMigrationDAG", mock.Anything, instances).
		Return(map[string]interface{}{"dag": "empty"}, nil)
	orchestrator.On("ExecuteMigrationDAG", mock.Anything, mock.Anything).
		Return(map[string]interface{}{"success": true}, nil)

	result, err := migrator.MigrateInstanceDisks(ctx, instances)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	orchestrator.AssertExpectations(t)
}
