package migrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

// mockDiskMigrator implements DiskMigrator for testing
type mockDiskMigrator struct {
	MigrateInstanceDisksFunc   func(ctx context.Context, migration *InstanceMigration) error
	MigrateInstanceDisksErr    error
	MigrateInstanceDisksCalled bool
	LastMigration              *InstanceMigration
	CallCount                  int
	ReturnErrors               []error
	CurrentCall                int
}

func (m *mockDiskMigrator) MigrateInstanceDisks(ctx context.Context, migration *InstanceMigration) error {
	m.MigrateInstanceDisksCalled = true
	m.LastMigration = migration
	m.CallCount++

	if m.MigrateInstanceDisksFunc != nil {
		return m.MigrateInstanceDisksFunc(ctx, migration)
	}

	if m.ReturnErrors != nil && m.CurrentCall < len(m.ReturnErrors) {
		err := m.ReturnErrors[m.CurrentCall]
		m.CurrentCall++
		return err
	}

	return m.MigrateInstanceDisksErr
}

func TestJobResult(t *testing.T) {
	result := &JobResult{
		JobID:        "test-job-1",
		InstanceName: "test-instance",
		Success:      true,
		ProcessedAt:  time.Now(),
	}

	assert.Equal(t, "test-job-1", result.JobID)
	assert.Equal(t, "test-instance", result.InstanceName)
	assert.True(t, result.Success)
	assert.NotZero(t, result.ProcessedAt)
}

func TestMigrationWorker_GetID(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	var wg sync.WaitGroup
	worker := NewMigrationWorker(42, queue, resultChan, mockMigrator, stateManager, &wg)

	assert.Equal(t, 42, worker.GetID())
}

func TestMigrationWorker_ProcessJob_Success(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	job := &MigrationJob{
		ID:       "test-job-1",
		Instance: instance,
		Config:   &Config{},
	}

	// Mock successful migration
	mockMigrator.MigrateInstanceDisksErr = nil

	var wg sync.WaitGroup
	worker := NewMigrationWorker(1, queue, resultChan, mockMigrator, stateManager, &wg)

	err := worker.ProcessJob(job)
	assert.NoError(t, err)

	// Check result
	select {
	case result := <-resultChan:
		assert.Equal(t, "test-job-1", result.JobID)
		assert.Equal(t, "test-instance", result.InstanceName)
		assert.True(t, result.Success)
		assert.Nil(t, result.Error)
		assert.NotNil(t, result.InstanceMigration)
		assert.Equal(t, MigrationStatusCompleted, result.InstanceMigration.Status)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected result not received")
	}

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}

func TestMigrationWorker_ProcessJob_Failure(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	job := &MigrationJob{
		ID:       "test-job-1",
		Instance: instance,
		Config:   &Config{},
	}

	expectedError := errors.New("migration failed")

	// Mock failed migration
	mockMigrator.MigrateInstanceDisksErr = expectedError

	var wg sync.WaitGroup
	worker := NewMigrationWorker(1, queue, resultChan, mockMigrator, stateManager, &wg)

	err := worker.ProcessJob(job)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Check result
	select {
	case result := <-resultChan:
		assert.Equal(t, "test-job-1", result.JobID)
		assert.Equal(t, "test-instance", result.InstanceName)
		assert.False(t, result.Success)
		assert.Equal(t, expectedError, result.Error)
		assert.NotNil(t, result.InstanceMigration)
		assert.Equal(t, MigrationStatusFailed, result.InstanceMigration.Status)
		assert.Len(t, result.InstanceMigration.Errors, 1)
		assert.Equal(t, ErrorTypePermanent, result.InstanceMigration.Errors[0].Type)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected result not received")
	}

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}

func TestMigrationWorker_Start_And_Stop(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	var wg sync.WaitGroup
	worker := NewMigrationWorker(1, queue, resultChan, mockMigrator, stateManager, &wg)

	// Start worker
	worker.Start()

	// Give worker time to register and start
	time.Sleep(50 * time.Millisecond)

	// Stop worker
	worker.Stop()

	// Wait for worker to finish
	wg.Wait()

	// Worker should have stopped cleanly
}

func TestMigrationWorker_JobProcessing(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	job := &MigrationJob{
		ID:         "test-job-1",
		Instance:   instance,
		Config:     &Config{},
		MaxRetries: 3,
	}

	// Mock successful migration
	mockMigrator.MigrateInstanceDisksErr = nil

	var wg sync.WaitGroup
	worker := NewMigrationWorker(1, queue, resultChan, mockMigrator, stateManager, &wg)

	// Start worker
	worker.Start()

	// Give worker time to register
	time.Sleep(50 * time.Millisecond)

	// Enqueue job
	err := queue.Enqueue(job)
	assert.NoError(t, err)

	// Wait for result
	select {
	case result := <-resultChan:
		assert.Equal(t, "test-job-1", result.JobID)
		assert.True(t, result.Success)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected result not received")
	}

	// Stop worker
	worker.Stop()
	wg.Wait()

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}

func TestMigrationWorker_JobRetry(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	resultChan := make(chan *JobResult, 10)
	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	job := &MigrationJob{
		ID:         "test-job-1",
		Instance:   instance,
		Config:     &Config{},
		MaxRetries: 2,
		Attempts:   0,
	}

	expectedError := errors.New("migration failed")

	// Mock failed migration twice, then success
	mockMigrator.ReturnErrors = []error{expectedError, expectedError, nil}

	var wg sync.WaitGroup
	worker := NewMigrationWorker(1, queue, resultChan, mockMigrator, stateManager, &wg)

	// Start worker
	worker.Start()

	// Give worker time to register
	time.Sleep(50 * time.Millisecond)

	// Enqueue job
	err := queue.Enqueue(job)
	assert.NoError(t, err)

	// Should receive results for failed attempts and final success
	resultsReceived := 0
	var lastResult *JobResult

	timeout := time.After(2 * time.Second)
	for resultsReceived < 3 {
		select {
		case result := <-resultChan:
			resultsReceived++
			lastResult = result
			assert.Equal(t, "test-job-1", result.JobID)
		case <-timeout:
			t.Fatalf("Expected 3 results, received %d", resultsReceived)
		}
	}

	// Last result should be successful
	assert.True(t, lastResult.Success)

	// Stop worker
	worker.Stop()
	wg.Wait()

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}

func TestWorkerPool_NewWorkerPool(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(5, queue, mockMigrator, stateManager)

	assert.NotNil(t, pool)
	assert.Equal(t, 5, pool.concurrency)
	assert.False(t, pool.IsStarted())
	assert.Equal(t, 0, pool.GetWorkerCount())
	assert.NotNil(t, pool.GetResultChannel())
}

func TestWorkerPool_Start(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(3, queue, mockMigrator, stateManager)

	err := pool.Start()
	assert.NoError(t, err)
	assert.True(t, pool.IsStarted())
	assert.Equal(t, 3, pool.GetWorkerCount())

	// Try to start again (should fail)
	err = pool.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Shutdown
	err = pool.Shutdown(3 * time.Second)
	assert.NoError(t, err)
	assert.False(t, pool.IsStarted())
}

func TestWorkerPool_Shutdown_Not_Started(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(3, queue, mockMigrator, stateManager)

	err := pool.Shutdown(1 * time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestWorkerPool_ProcessJobs(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(2, queue, mockMigrator, stateManager)

	instance1 := &computepb.Instance{
		Name:     proto.String("instance-1"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/instance-1"),
		Status:   proto.String("RUNNING"),
	}

	instance2 := &computepb.Instance{
		Name:     proto.String("instance-2"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/instance-2"),
		Status:   proto.String("RUNNING"),
	}

	job1 := &MigrationJob{
		ID:       "job-1",
		Instance: instance1,
		Config:   &Config{},
	}

	job2 := &MigrationJob{
		ID:       "job-2",
		Instance: instance2,
		Config:   &Config{},
	}

	// Mock successful migrations
	mockMigrator.MigrateInstanceDisksErr = nil

	// Start pool
	err := pool.Start()
	assert.NoError(t, err)

	// Give workers time to start and register
	time.Sleep(100 * time.Millisecond)

	// Enqueue jobs
	err = queue.Enqueue(job1)
	assert.NoError(t, err)
	err = queue.Enqueue(job2)
	assert.NoError(t, err)

	// Wait for results
	resultsReceived := 0
	resultsChan := pool.GetResultChannel()

	timeout := time.After(2 * time.Second)
	for resultsReceived < 2 {
		select {
		case result := <-resultsChan:
			resultsReceived++
			assert.True(t, result.Success)
			assert.Contains(t, []string{"job-1", "job-2"}, result.JobID)
		case <-timeout:
			t.Fatalf("Expected 2 results, received %d", resultsReceived)
		}
	}

	// Shutdown
	err = pool.Shutdown(3 * time.Second)
	assert.NoError(t, err)

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}

func TestWorkerPool_DrainResults(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(1, queue, mockMigrator, stateManager)

	// Manually add some results to the channel
	result1 := &JobResult{JobID: "job-1", Success: true}
	result2 := &JobResult{JobID: "job-2", Success: false}

	pool.resultChan <- result1
	pool.resultChan <- result2

	results := pool.DrainResults()
	assert.Len(t, results, 2)

	// Check that results are in the slice (order might vary)
	jobIDs := make(map[string]bool)
	for _, r := range results {
		jobIDs[r.JobID] = true
	}
	assert.True(t, jobIDs["job-1"])
	assert.True(t, jobIDs["job-2"])
}

func TestWorkerPool_ConcurrentOperations(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(50)
	defer queue.Shutdown()

	mockMigrator := &mockDiskMigrator{}
	stateManager := NewInstanceStateManager()

	pool := NewWorkerPool(5, queue, mockMigrator, stateManager)

	// Mock all migrations as successful
	mockMigrator.MigrateInstanceDisksErr = nil

	// Start pool
	err := pool.Start()
	assert.NoError(t, err)

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	// Create and enqueue multiple jobs concurrently
	numJobs := 20
	var enqueueWg sync.WaitGroup

	for i := 0; i < numJobs; i++ {
		enqueueWg.Add(1)
		go func(jobID int) {
			defer enqueueWg.Done()

			instance := &computepb.Instance{
				Name:     proto.String("instance-" + string(rune(jobID+'0'))),
				Zone:     proto.String("projects/test-project/zones/us-west1-a"),
				SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/instance-" + string(rune(jobID+'0'))),
				Status:   proto.String("RUNNING"),
			}

			job := &MigrationJob{
				ID:       "job-" + string(rune(jobID+'0')),
				Instance: instance,
				Config:   &Config{},
			}

			err := queue.Enqueue(job)
			assert.NoError(t, err)
		}(i)
	}

	enqueueWg.Wait()

	// Wait for all results
	resultsReceived := 0
	resultsChan := pool.GetResultChannel()

	timeout := time.After(5 * time.Second)
	for resultsReceived < numJobs {
		select {
		case result := <-resultsChan:
			resultsReceived++
			assert.True(t, result.Success)
		case <-timeout:
			t.Fatalf("Expected %d results, received %d", numJobs, resultsReceived)
		}
	}

	// Shutdown
	err = pool.Shutdown(2 * time.Second)
	assert.NoError(t, err)

	assert.True(t, mockMigrator.MigrateInstanceDisksCalled)
}
