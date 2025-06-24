package migrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
)

// JobResult represents the result of a migration job
type JobResult struct {
	JobID             string
	InstanceName      string
	Success           bool
	Error             error
	InstanceMigration *InstanceMigration
	ProcessedAt       time.Time
}

// MigrationWorker implements the Worker interface
type MigrationWorker struct {
	id                   int
	jobQueue             *JobQueue
	resultChan           chan *JobResult
	diskMigrator         DiskMigratorInterface
	instanceStateManager *InstanceStateManager
	ctx                  context.Context
	cancel               context.CancelFunc
	wg                   *sync.WaitGroup
}

// NewMigrationWorker creates a new migration worker
func NewMigrationWorker(
	id int,
	jobQueue *JobQueue,
	resultChan chan *JobResult,
	diskMigrator DiskMigratorInterface,
	instanceStateManager *InstanceStateManager,
	wg *sync.WaitGroup,
) *MigrationWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &MigrationWorker{
		id:                   id,
		jobQueue:             jobQueue,
		resultChan:           resultChan,
		diskMigrator:         diskMigrator,
		instanceStateManager: instanceStateManager,
		ctx:                  ctx,
		cancel:               cancel,
		wg:                   wg,
	}
}

// GetID returns the worker ID
func (w *MigrationWorker) GetID() int {
	return w.id
}

// ProcessJob processes a migration job
func (w *MigrationWorker) ProcessJob(job *MigrationJob) error {
	logger.Op.WithFields(map[string]interface{}{
		"workerID":     w.id,
		"jobID":        job.ID,
		"instanceName": job.GetInstanceName(),
	}).Info("Worker processing migration job")

	result := &JobResult{
		JobID:        job.ID,
		InstanceName: job.GetInstanceName(),
		ProcessedAt:  time.Now(),
	}

	// Store initial instance state
	initialState := w.instanceStateManager.GetState(job.Instance)

	// Create instance migration
	instanceMigration := &InstanceMigration{
		Instance:     job.Instance,
		InitialState: initialState,
		Status:       MigrationStatusInProgress,
	}
	result.InstanceMigration = instanceMigration

	// Process the migration
	err := w.diskMigrator.MigrateInstanceDisks(w.ctx, instanceMigration)
	if err != nil {
		result.Success = false
		result.Error = err
		instanceMigration.Status = MigrationStatusFailed
		instanceMigration.Errors = append(instanceMigration.Errors, MigrationError{
			Type:   ErrorTypePermanent,
			Phase:  PhaseMigration,
			Target: job.GetInstanceName(),
			Cause:  err,
		})

		logger.Op.WithFields(map[string]interface{}{
			"workerID":     w.id,
			"jobID":        job.ID,
			"instanceName": job.GetInstanceName(),
			"error":        err.Error(),
		}).Error("Migration job failed")
	} else {
		result.Success = true
		instanceMigration.Status = MigrationStatusCompleted

		logger.Op.WithFields(map[string]interface{}{
			"workerID":     w.id,
			"jobID":        job.ID,
			"instanceName": job.GetInstanceName(),
		}).Info("Migration job completed successfully")
	}

	// Send result to result channel
	select {
	case w.resultChan <- result:
		// Result sent successfully
	case <-w.ctx.Done():
		return fmt.Errorf("worker %d: context cancelled while sending result", w.id)
	default:
		logger.Op.WithFields(map[string]interface{}{
			"workerID": w.id,
			"jobID":    job.ID,
		}).Warn("Result channel full, dropping result")
	}

	return err
}

// Start begins the worker's job processing loop
func (w *MigrationWorker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			logger.Op.WithFields(map[string]interface{}{
				"workerID": w.id,
			}).Info("Worker stopped")
		}()

		logger.Op.WithFields(map[string]interface{}{
			"workerID": w.id,
		}).Info("Worker started")

		// Register this worker with the job queue
		if err := w.jobQueue.RegisterWorker(w); err != nil {
			logger.Op.WithFields(map[string]interface{}{
				"workerID": w.id,
				"error":    err.Error(),
			}).Error("Failed to register worker")
			return
		}

		for {
			select {
			case <-w.ctx.Done():
				logger.Op.WithFields(map[string]interface{}{
					"workerID": w.id,
				}).Info("Worker shutting down")
				return
			default:
				// Try to get a job from the queue with timeout
				select {
				case <-w.ctx.Done():
					return
				default:
					// Use a timeout channel to avoid blocking indefinitely
					jobChan := make(chan *MigrationJob, 1)
					errChan := make(chan error, 1)

					go func() {
						job, err := w.jobQueue.Dequeue()
						if err != nil {
							errChan <- err
						} else {
							jobChan <- job
						}
					}()

					var job *MigrationJob
					var err error

					select {
					case <-w.ctx.Done():
						return
					case job = <-jobChan:
						// Got a job successfully
					case err = <-errChan:
						if w.ctx.Err() != nil {
							// Context cancelled, normal shutdown
							return
						}
						logger.Op.WithFields(map[string]interface{}{
							"workerID": w.id,
							"error":    err.Error(),
						}).Debug("Failed to dequeue job")
						// Brief pause before retrying
						select {
						case <-w.ctx.Done():
							return
						case <-time.After(100 * time.Millisecond):
							continue
						}
					case <-time.After(1 * time.Second):
						// Timeout - check context and continue
						select {
						case <-w.ctx.Done():
							return
						default:
							continue
						}
					}

					if job == nil {
						continue
					}

					// Process the job
					err = w.ProcessJob(job)
					if err != nil && job.CanRetry() {
						job.IncrementAttempts()
						logger.Op.WithFields(map[string]interface{}{
							"workerID":   w.id,
							"jobID":      job.ID,
							"attempts":   job.Attempts,
							"maxRetries": job.MaxRetries,
						}).Info("Retrying failed job")

						// Re-enqueue for retry
						if retryErr := w.jobQueue.Enqueue(job); retryErr != nil {
							logger.Op.WithFields(map[string]interface{}{
								"workerID": w.id,
								"jobID":    job.ID,
								"error":    retryErr.Error(),
							}).Error("Failed to re-enqueue job for retry")
						}
					}
				}
			}
		}
	}()
}

// Stop gracefully stops the worker
func (w *MigrationWorker) Stop() {
	w.cancel()
}

// WorkerPool manages a pool of migration workers
type WorkerPool struct {
	workers              []*MigrationWorker
	jobQueue             *JobQueue
	resultChan           chan *JobResult
	concurrency          int
	shutdownChan         chan struct{}
	wg                   sync.WaitGroup
	diskMigrator         DiskMigratorInterface
	instanceStateManager *InstanceStateManager
	started              bool
	mu                   sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
	concurrency int,
	jobQueue *JobQueue,
	diskMigrator DiskMigratorInterface,
	instanceStateManager *InstanceStateManager,
) *WorkerPool {
	return &WorkerPool{
		workers:              make([]*MigrationWorker, 0, concurrency),
		jobQueue:             jobQueue,
		resultChan:           make(chan *JobResult, concurrency*10),
		concurrency:          concurrency,
		shutdownChan:         make(chan struct{}),
		diskMigrator:         diskMigrator,
		instanceStateManager: instanceStateManager,
		started:              false,
	}
}

// Start initializes and starts all workers in the pool
func (p *WorkerPool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("worker pool already started")
	}

	logger.Op.WithFields(map[string]interface{}{
		"concurrency": p.concurrency,
	}).Info("Starting worker pool")

	// Create and start workers
	for i := 0; i < p.concurrency; i++ {
		worker := NewMigrationWorker(
			i+1,
			p.jobQueue,
			p.resultChan,
			p.diskMigrator,
			p.instanceStateManager,
			&p.wg,
		)
		p.workers = append(p.workers, worker)
		worker.Start()
	}

	p.started = true
	logger.Op.WithFields(map[string]interface{}{
		"workerCount": len(p.workers),
	}).Info("Worker pool started successfully")

	return nil
}

// Shutdown gracefully shuts down the worker pool
func (p *WorkerPool) Shutdown(timeout time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return fmt.Errorf("worker pool not started")
	}

	logger.Op.WithFields(map[string]interface{}{
		"timeout": timeout.String(),
	}).Info("Shutting down worker pool")

	// Signal shutdown
	close(p.shutdownChan)

	// Stop all workers
	for _, worker := range p.workers {
		worker.Stop()
	}

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Op.Info("All workers stopped gracefully")
	case <-time.After(timeout):
		logger.Op.Warn("Worker pool shutdown timed out")
		return fmt.Errorf("worker pool shutdown timed out after %v", timeout)
	}

	// Close result channel
	close(p.resultChan)
	p.started = false

	logger.Op.Info("Worker pool shutdown completed")
	return nil
}

// GetResultChannel returns the channel for receiving migration results
func (p *WorkerPool) GetResultChannel() <-chan *JobResult {
	return p.resultChan
}

// IsStarted returns whether the worker pool is currently started
func (p *WorkerPool) IsStarted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

// GetWorkerCount returns the number of workers in the pool
func (p *WorkerPool) GetWorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.workers)
}

// DrainResults returns all remaining results from the result channel
func (p *WorkerPool) DrainResults() []*JobResult {
	var results []*JobResult

	for {
		select {
		case result := <-p.resultChan:
			results = append(results, result)
		default:
			return results
		}
	}
}
