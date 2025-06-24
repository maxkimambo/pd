package migrator

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

// mockWorker implements the Worker interface for testing
type mockWorker struct {
	id           int
	processCount int
	processError error
}

func (w *mockWorker) GetID() int {
	return w.id
}

func (w *mockWorker) ProcessJob(job *MigrationJob) error {
	w.processCount++
	return w.processError
}

func TestJobPriority_String(t *testing.T) {
	tests := []struct {
		priority JobPriority
		expected string
	}{
		{LowPriority, "low"},
		{MediumPriority, "medium"},
		{HighPriority, "high"},
		{JobPriority(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.priority.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrationJob_GetInstanceName(t *testing.T) {
	t.Run("With instance", func(t *testing.T) {
		instance := &computepb.Instance{
			Name: proto.String("test-instance"),
		}
		job := &MigrationJob{
			Instance: instance,
		}
		assert.Equal(t, "test-instance", job.GetInstanceName())
	})

	t.Run("Without instance", func(t *testing.T) {
		job := &MigrationJob{
			Instance: nil,
		}
		assert.Equal(t, "", job.GetInstanceName())
	})
}

func TestMigrationJob_CanRetry(t *testing.T) {
	tests := []struct {
		name       string
		attempts   int
		maxRetries int
		expected   bool
	}{
		{"Can retry", 0, 3, true},
		{"Can retry - on limit", 2, 3, true},
		{"Cannot retry - exceeded", 3, 3, false},
		{"Cannot retry - over limit", 4, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &MigrationJob{
				Attempts:   tt.attempts,
				MaxRetries: tt.maxRetries,
			}
			assert.Equal(t, tt.expected, job.CanRetry())
		})
	}
}

func TestMigrationJob_IncrementAttempts(t *testing.T) {
	job := &MigrationJob{
		Attempts: 0,
	}

	assert.Equal(t, 0, job.Attempts)
	job.IncrementAttempts()
	assert.Equal(t, 1, job.Attempts)
	job.IncrementAttempts()
	assert.Equal(t, 2, job.Attempts)
}

func TestQueueMetrics(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	metrics := NewQueueMetrics()

	assert.Equal(t, int64(0), metrics.GetJobsEnqueued())
	assert.Equal(t, int64(0), metrics.GetJobsDequeued())
	assert.Equal(t, int64(0), metrics.GetJobsDropped())
	assert.Equal(t, int64(0), metrics.GetQueueSize())
}

func TestJobQueue_NewJobQueue(t *testing.T) {
	queue := NewJobQueue(10)

	assert.NotNil(t, queue)
	assert.NotNil(t, queue.jobs)
	assert.NotNil(t, queue.workers)
	assert.NotNil(t, queue.metrics)
	assert.NotNil(t, queue.shutdownCh)
	assert.False(t, queue.closed)
	assert.Equal(t, 0, queue.Size())
}

func TestJobQueue_EnqueueDequeue(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	instance := &computepb.Instance{
		Name: proto.String("test-instance"),
	}

	job := &MigrationJob{
		ID:         "job-1",
		Instance:   instance,
		Priority:   MediumPriority,
		CreatedAt:  time.Now(),
		MaxRetries: 3,
	}

	t.Run("Enqueue and dequeue job", func(t *testing.T) {
		err := queue.Enqueue(job)
		assert.NoError(t, err)
		assert.Equal(t, 1, queue.Size())

		dequeuedJob, err := queue.Dequeue()
		assert.NoError(t, err)
		assert.Equal(t, job.ID, dequeuedJob.ID)
		assert.Equal(t, job.GetInstanceName(), dequeuedJob.GetInstanceName())
		assert.Equal(t, 0, queue.Size())
	})

	t.Run("Queue metrics", func(t *testing.T) {
		// Reset queue
		queue = NewJobQueue(10)
		defer queue.Shutdown()

		err := queue.Enqueue(job)
		assert.NoError(t, err)

		metrics := queue.GetMetrics()
		assert.Equal(t, int64(1), metrics.GetJobsEnqueued())
		assert.Equal(t, int64(1), metrics.GetQueueSize())

		_, err = queue.Dequeue()
		assert.NoError(t, err)

		metrics = queue.GetMetrics()
		assert.Equal(t, int64(1), metrics.GetJobsDequeued())
		assert.Equal(t, int64(0), metrics.GetQueueSize())
	})
}

func TestJobQueue_FullQueue(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(2) // Small queue size
	defer queue.Shutdown()

	instance := &computepb.Instance{
		Name: proto.String("test-instance"),
	}

	// Fill the queue
	for i := 0; i < 2; i++ {
		job := &MigrationJob{
			ID:       fmt.Sprintf("job-%d", i),
			Instance: instance,
		}
		err := queue.Enqueue(job)
		assert.NoError(t, err)
	}

	// Try to enqueue one more (should fail)
	extraJob := &MigrationJob{
		ID:       "extra-job",
		Instance: instance,
	}
	err := queue.Enqueue(extraJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue is full")

	metrics := queue.GetMetrics()
	assert.Equal(t, int64(1), metrics.GetJobsDropped())
}

func TestJobQueue_WorkerPool(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	worker1 := &mockWorker{id: 1}
	worker2 := &mockWorker{id: 2}

	t.Run("Register and retrieve workers", func(t *testing.T) {
		err := queue.RegisterWorker(worker1)
		assert.NoError(t, err)

		err = queue.RegisterWorker(worker2)
		assert.NoError(t, err)

		// Get a worker
		retrievedWorker, err := queue.GetAvailableWorker()
		assert.NoError(t, err)
		assert.NotNil(t, retrievedWorker)

		// Return the worker
		err = queue.ReturnWorker(retrievedWorker)
		assert.NoError(t, err)
	})
}

func TestJobQueue_Shutdown(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)

	instance := &computepb.Instance{
		Name: proto.String("test-instance"),
	}

	job := &MigrationJob{
		ID:       "job-1",
		Instance: instance,
	}

	// Enqueue a job
	err := queue.Enqueue(job)
	assert.NoError(t, err)

	// Shutdown the queue
	queue.Shutdown()

	// Verify queue is closed
	assert.True(t, queue.IsClosed())

	// Try to enqueue after shutdown (should fail)
	err = queue.Enqueue(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue is closed")

	// Try to dequeue after shutdown (should fail)
	_, err = queue.Dequeue()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue is shutting down")
}

func TestJobQueue_DrainJobs(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(10)
	defer queue.Shutdown()

	instance := &computepb.Instance{
		Name: proto.String("test-instance"),
	}

	// Add some jobs
	jobs := []*MigrationJob{
		{ID: "job-1", Instance: instance},
		{ID: "job-2", Instance: instance},
		{ID: "job-3", Instance: instance},
	}

	for _, job := range jobs {
		err := queue.Enqueue(job)
		assert.NoError(t, err)
	}

	assert.Equal(t, 3, queue.Size())

	// Drain jobs
	drainedJobs := queue.DrainJobs()
	assert.Len(t, drainedJobs, 3)
	assert.Equal(t, 0, queue.Size())

	// Verify job IDs
	jobIDs := make(map[string]bool)
	for _, job := range drainedJobs {
		jobIDs[job.ID] = true
	}
	assert.True(t, jobIDs["job-1"])
	assert.True(t, jobIDs["job-2"])
	assert.True(t, jobIDs["job-3"])
}

func TestJobQueue_ConcurrentAccess(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(100)
	defer queue.Shutdown()

	instance := &computepb.Instance{
		Name: proto.String("test-instance"),
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	jobsPerGoroutine := 10

	// Concurrent enqueuing
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < jobsPerGoroutine; j++ {
				job := &MigrationJob{
					ID:       fmt.Sprintf("worker-%d-job-%d", workerID, j),
					Instance: instance,
				}
				err := queue.Enqueue(job)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all jobs were enqueued
	assert.Equal(t, numGoroutines*jobsPerGoroutine, queue.Size())

	// Concurrent dequeuing
	wg.Add(numGoroutines)
	dequeuedCount := int64(0)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < jobsPerGoroutine; j++ {
				_, err := queue.Dequeue()
				if err == nil {
					atomic.AddInt64(&dequeuedCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify all jobs were dequeued
	assert.Equal(t, int64(numGoroutines*jobsPerGoroutine), dequeuedCount)
	assert.Equal(t, 0, queue.Size())
}

func TestJobQueue_WorkerPoolConcurrent(t *testing.T) {
	// Setup logger for tests
	logger.Setup(false, false, false)

	queue := NewJobQueue(50)
	defer queue.Shutdown()

	var wg sync.WaitGroup
	numWorkers := 5

	// Register workers concurrently
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			worker := &mockWorker{id: id}
			err := queue.RegisterWorker(worker)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Get and return workers concurrently
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			worker, err := queue.GetAvailableWorker()
			assert.NoError(t, err)
			assert.NotNil(t, worker)

			// Return the worker
			err = queue.ReturnWorker(worker)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()
}
