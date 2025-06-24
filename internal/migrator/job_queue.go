package migrator

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/logger"
)

// JobPriority defines the priority levels for migration jobs
type JobPriority int

const (
	LowPriority JobPriority = iota
	MediumPriority
	HighPriority
)

// String returns the string representation of JobPriority
func (p JobPriority) String() string {
	switch p {
	case LowPriority:
		return "low"
	case MediumPriority:
		return "medium"
	case HighPriority:
		return "high"
	default:
		return "unknown"
	}
}

// MigrationJob represents a single instance migration task
type MigrationJob struct {
	ID         string
	Instance   *computepb.Instance
	Config     *Config
	Priority   JobPriority
	CreatedAt  time.Time
	Attempts   int
	MaxRetries int
}

// GetInstanceName returns the name of the instance for this job
func (j *MigrationJob) GetInstanceName() string {
	if j.Instance == nil {
		return ""
	}
	return j.Instance.GetName()
}

// CanRetry returns true if the job can be retried
func (j *MigrationJob) CanRetry() bool {
	return j.Attempts < j.MaxRetries
}

// IncrementAttempts increments the attempt counter
func (j *MigrationJob) IncrementAttempts() {
	j.Attempts++
}

// QueueMetrics tracks metrics for the job queue
type QueueMetrics struct {
	jobsEnqueued int64
	jobsDequeued int64
	jobsDropped  int64
	queueSize    int64
}

// NewQueueMetrics creates a new QueueMetrics instance
func NewQueueMetrics() *QueueMetrics {
	return &QueueMetrics{}
}

// GetJobsEnqueued returns the number of jobs enqueued
func (m *QueueMetrics) GetJobsEnqueued() int64 {
	return atomic.LoadInt64(&m.jobsEnqueued)
}

// GetJobsDequeued returns the number of jobs dequeued
func (m *QueueMetrics) GetJobsDequeued() int64 {
	return atomic.LoadInt64(&m.jobsDequeued)
}

// GetJobsDropped returns the number of jobs dropped
func (m *QueueMetrics) GetJobsDropped() int64 {
	return atomic.LoadInt64(&m.jobsDropped)
}

// GetQueueSize returns the current queue size
func (m *QueueMetrics) GetQueueSize() int64 {
	return atomic.LoadInt64(&m.queueSize)
}

// Worker represents a worker that can process migration jobs
type Worker interface {
	GetID() int
	ProcessJob(job *MigrationJob) error
}

// JobQueue provides a thread-safe queue for migration jobs
type JobQueue struct {
	jobs       chan *MigrationJob
	workers    chan Worker
	mu         sync.RWMutex
	metrics    *QueueMetrics
	shutdownCh chan struct{}
	closed     bool
}

// NewJobQueue creates a new JobQueue with the specified queue size
func NewJobQueue(queueSize int) *JobQueue {
	return &JobQueue{
		jobs:       make(chan *MigrationJob, queueSize),
		workers:    make(chan Worker, queueSize),
		metrics:    NewQueueMetrics(),
		shutdownCh: make(chan struct{}),
		closed:     false,
	}
}

// Enqueue adds a job to the queue
func (q *JobQueue) Enqueue(job *MigrationJob) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		atomic.AddInt64(&q.metrics.jobsDropped, 1)
		return fmt.Errorf("queue is closed")
	}

	select {
	case q.jobs <- job:
		atomic.AddInt64(&q.metrics.jobsEnqueued, 1)
		atomic.AddInt64(&q.metrics.queueSize, 1)
		logger.Op.WithFields(map[string]interface{}{
			"jobID":    job.ID,
			"instance": job.GetInstanceName(),
			"priority": job.Priority.String(),
		}).Debug("Job enqueued")
		return nil
	case <-q.shutdownCh:
		atomic.AddInt64(&q.metrics.jobsDropped, 1)
		return fmt.Errorf("queue is shutting down")
	default:
		atomic.AddInt64(&q.metrics.jobsDropped, 1)
		return fmt.Errorf("queue is full")
	}
}

// Dequeue removes and returns a job from the queue
func (q *JobQueue) Dequeue() (*MigrationJob, error) {
	// Check if queue is closed first
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return nil, fmt.Errorf("queue is shutting down")
	}
	q.mu.RUnlock()

	select {
	case job, ok := <-q.jobs:
		if !ok {
			return nil, fmt.Errorf("queue is shutting down")
		}
		atomic.AddInt64(&q.metrics.jobsDequeued, 1)
		atomic.AddInt64(&q.metrics.queueSize, -1)
		logger.Op.WithFields(map[string]interface{}{
			"jobID":    job.ID,
			"instance": job.GetInstanceName(),
			"priority": job.Priority.String(),
		}).Debug("Job dequeued")
		return job, nil
	case <-q.shutdownCh:
		return nil, fmt.Errorf("queue is shutting down")
	}
}

// RegisterWorker registers a worker with the queue
func (q *JobQueue) RegisterWorker(worker Worker) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return fmt.Errorf("queue is closed")
	}

	select {
	case q.workers <- worker:
		logger.Op.WithFields(map[string]interface{}{
			"workerID": worker.GetID(),
		}).Debug("Worker registered")
		return nil
	default:
		return fmt.Errorf("worker pool is full")
	}
}

// GetAvailableWorker returns an available worker from the pool
func (q *JobQueue) GetAvailableWorker() (Worker, error) {
	select {
	case worker := <-q.workers:
		logger.Op.WithFields(map[string]interface{}{
			"workerID": worker.GetID(),
		}).Debug("Worker retrieved")
		return worker, nil
	case <-q.shutdownCh:
		return nil, fmt.Errorf("queue is shutting down")
	}
}

// ReturnWorker returns a worker to the pool
func (q *JobQueue) ReturnWorker(worker Worker) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return fmt.Errorf("queue is closed")
	}

	select {
	case q.workers <- worker:
		logger.Op.WithFields(map[string]interface{}{
			"workerID": worker.GetID(),
		}).Debug("Worker returned to pool")
		return nil
	default:
		return fmt.Errorf("worker pool is full")
	}
}

// Size returns the current number of jobs in the queue
func (q *JobQueue) Size() int {
	return len(q.jobs)
}

// IsClosed returns true if the queue is closed
func (q *JobQueue) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// GetMetrics returns a copy of the current queue metrics
func (q *JobQueue) GetMetrics() QueueMetrics {
	return QueueMetrics{
		jobsEnqueued: q.metrics.GetJobsEnqueued(),
		jobsDequeued: q.metrics.GetJobsDequeued(),
		jobsDropped:  q.metrics.GetJobsDropped(),
		queueSize:    q.metrics.GetQueueSize(),
	}
}

// Shutdown gracefully shuts down the queue
func (q *JobQueue) Shutdown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	logger.Op.Info("Shutting down job queue")
	q.closed = true
	close(q.shutdownCh)
	close(q.jobs)
	close(q.workers)
}

// DrainJobs returns all remaining jobs in the queue
func (q *JobQueue) DrainJobs() []*MigrationJob {
	var jobs []*MigrationJob

	// Drain the jobs channel
	for {
		select {
		case job := <-q.jobs:
			jobs = append(jobs, job)
			atomic.AddInt64(&q.metrics.queueSize, -1)
		default:
			return jobs
		}
	}
}
