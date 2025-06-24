package migrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// ParallelMigrationConfig contains configuration for the parallel migration process
type ParallelMigrationConfig struct {
	// Base migration configuration
	BaseConfig *Config
	
	// Parallelism settings
	MaxConcurrency int
	QueueSize      int
	
	// Timeout settings
	WorkerStartTimeout time.Duration
	JobTimeout         time.Duration
	ShutdownTimeout    time.Duration
	
	// Progress reporting
	ProgressReportInterval time.Duration
	EnableProgressReporting bool
}

// MigrationSummary contains the final results of a parallel migration operation
type MigrationSummary struct {
	// Overall statistics
	TotalJobs         int
	SuccessfulJobs    int
	FailedJobsCount   int
	CompletionPercent float64
	
	// Timing information
	StartTime       time.Time
	EndTime         time.Time
	TotalDuration   time.Duration
	AverageDuration time.Duration
	
	// Instance and disk statistics
	TotalInstances      int
	SuccessfulInstances int
	FailedInstances     int
	TotalDisks          int
	SuccessfulDisks     int
	FailedDisks         int
	
	// Detailed results
	JobResults      []*JobResult
	FailedJobs      []*JobResult
	RetryableJobs   []*JobResult
	
	// Performance metrics
	ThroughputJobsPerSecond     float64
	ThroughputInstancesPerSecond float64
	PeakConcurrency             int
	AverageQueueDepth           float64
}

// ParallelMigrationCoordinator manages the overall parallel migration process
type ParallelMigrationCoordinator struct {
	// Core components
	discoveryService *InstanceDiscovery
	workerPool       *WorkerPool
	jobQueue         *JobQueue
	
	// Resource management
	resourceLocker  *ResourceLocker
	progressTracker *ProgressTracker
	
	// Configuration
	config *ParallelMigrationConfig
	
	// State management
	mu               sync.RWMutex
	isStarted        bool
	isShutdown       bool
	startTime        time.Time
	endTime          time.Time
	
	// Results collection
	results          []*JobResult
	resultsMu        sync.Mutex
	
	// Control channels
	shutdownCh       chan struct{}
	doneCh           chan struct{}
	progressReportCh chan struct{}
}

// NewParallelMigrationCoordinator creates a new parallel migration coordinator
func NewParallelMigrationCoordinator(config *ParallelMigrationConfig) (*ParallelMigrationCoordinator, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	
	if config.BaseConfig == nil {
		return nil, fmt.Errorf("base config cannot be nil")
	}
	
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 4 // Default concurrency
	}
	
	if config.QueueSize <= 0 {
		config.QueueSize = config.MaxConcurrency * 10 // Default queue size
	}
	
	// Set default timeouts
	if config.WorkerStartTimeout == 0 {
		config.WorkerStartTimeout = 30 * time.Second
	}
	if config.JobTimeout == 0 {
		config.JobTimeout = 10 * time.Minute
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 2 * time.Minute
	}
	if config.ProgressReportInterval == 0 {
		config.ProgressReportInterval = 30 * time.Second
	}

	// Create GCP clients
	ctx := context.Background()
	clients, err := gcp.NewClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP clients: %w", err)
	}

	// Create core components
	jobQueue := NewJobQueue(config.QueueSize)
	resourceLocker := NewResourceLocker()
	progressTracker := NewProgressTracker()
	
	// Create discovery service
	discoveryService := NewInstanceDiscovery(clients.ComputeClient, clients.DiskClient)
	
	// Create disk migrator for workers
	diskMigrator := NewDiskMigrator(clients.ComputeClient, clients.DiskClient, clients.SnapshotClient, config.BaseConfig)
	instanceStateManager := NewInstanceStateManager()
	
	// Create worker pool
	workerPool := NewWorkerPool(config.MaxConcurrency, jobQueue, diskMigrator, instanceStateManager)

	coordinator := &ParallelMigrationCoordinator{
		discoveryService: discoveryService,
		workerPool:       workerPool,
		jobQueue:         jobQueue,
		resourceLocker:   resourceLocker,
		progressTracker:  progressTracker,
		config:           config,
		results:          make([]*JobResult, 0),
		shutdownCh:       make(chan struct{}),
		doneCh:           make(chan struct{}),
		progressReportCh: make(chan struct{}, 1),
	}

	return coordinator, nil
}

// Start initializes and starts all components of the migration process
func (c *ParallelMigrationCoordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isStarted {
		return fmt.Errorf("coordinator already started")
	}

	if c.isShutdown {
		return fmt.Errorf("coordinator has been shutdown")
	}

	logger.Op.WithFields(map[string]interface{}{
		"maxConcurrency": c.config.MaxConcurrency,
		"queueSize":      c.config.QueueSize,
	}).Info("Starting parallel migration coordinator")

	c.startTime = time.Now()

	// Start the worker pool
	workerStartCtx, cancel := context.WithTimeout(ctx, c.config.WorkerStartTimeout)
	defer cancel()

	if err := c.workerPool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Start result collection goroutine
	go c.collectResults()

	// Start progress reporting if enabled
	if c.config.EnableProgressReporting {
		go c.reportProgress()
	}

	// Discover instances and queue jobs
	if err := c.DiscoverAndQueueJobs(workerStartCtx); err != nil {
		// If discovery fails, clean up started components
		c.shutdown()
		return fmt.Errorf("failed to discover and queue jobs: %w", err)
	}

	c.isStarted = true

	logger.Op.Info("Parallel migration coordinator started successfully")
	return nil
}

// DiscoverAndQueueJobs discovers instances and creates prioritized migration jobs
func (c *ParallelMigrationCoordinator) DiscoverAndQueueJobs(ctx context.Context) error {
	logger.Op.Info("Discovering instances for migration")

	// Discover instances using the discovery service
	instanceMigrations, err := c.discoveryService.DiscoverInstances(ctx, c.config.BaseConfig)
	if err != nil {
		return fmt.Errorf("failed to discover instances: %w", err)
	}

	if len(instanceMigrations) == 0 {
		logger.Op.Warn("No instances found for migration")
		close(c.doneCh)
		return nil
	}

	logger.Op.WithFields(map[string]interface{}{
		"instanceCount": len(instanceMigrations),
	}).Info("Discovered instances for migration")

	// Create and prioritize jobs
	jobs := make([]*MigrationJob, 0, len(instanceMigrations))
	for i, migration := range instanceMigrations {
		job := &MigrationJob{
			ID:         fmt.Sprintf("job-%d-%s", i+1, migration.Instance.GetName()),
			Instance:   migration.Instance,
			Config:     c.config.BaseConfig,
			Priority:   c.calculateJobPriority(migration),
			CreatedAt:  time.Now(),
			Attempts:   0,
			MaxRetries: 3, // Default retry count
		}
		jobs = append(jobs, job)
	}

	// Sort jobs by priority (high priority first)
	c.sortJobsByPriority(jobs)

	// Enqueue all jobs
	queuedCount := 0
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.jobQueue.Enqueue(job); err != nil {
				logger.Op.WithFields(map[string]interface{}{
					"jobID":    job.ID,
					"instance": job.GetInstanceName(),
					"error":    err.Error(),
				}).Error("Failed to enqueue job")
				continue
			}
			queuedCount++
		}
	}

	logger.Op.WithFields(map[string]interface{}{
		"totalJobs":   len(jobs),
		"queuedJobs":  queuedCount,
		"failedJobs":  len(jobs) - queuedCount,
	}).Info("Finished queuing migration jobs")

	return nil
}

// calculateJobPriority determines the priority of a migration job
func (c *ParallelMigrationCoordinator) calculateJobPriority(migration *InstanceMigration) JobPriority {
	// For now, use a simple priority scheme
	// In practice, this could consider factors like:
	// - Instance size/importance
	// - Disk size
	// - Business criticality
	// - Time constraints
	
	diskCount := len(migration.Disks)
	
	switch {
	case diskCount >= 5:
		return HighPriority // Many disks = higher priority
	case diskCount >= 2:
		return MediumPriority
	default:
		return LowPriority
	}
}

// sortJobsByPriority sorts jobs by priority (high to low)
func (c *ParallelMigrationCoordinator) sortJobsByPriority(jobs []*MigrationJob) {
	// Simple bubble sort by priority
	for i := 0; i < len(jobs)-1; i++ {
		for j := 0; j < len(jobs)-i-1; j++ {
			if jobs[j].Priority < jobs[j+1].Priority {
				jobs[j], jobs[j+1] = jobs[j+1], jobs[j]
			}
		}
	}
}

// collectResults collects results from the worker pool
func (c *ParallelMigrationCoordinator) collectResults() {
	defer close(c.doneCh)
	
	resultChan := c.workerPool.GetResultChannel()
	
	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				// Channel closed, all workers done
				return
			}
			
			c.resultsMu.Lock()
			c.results = append(c.results, result)
			c.resultsMu.Unlock()
			
			// Trigger progress report
			select {
			case c.progressReportCh <- struct{}{}:
			default:
			}
			
		case <-c.shutdownCh:
			return
		}
	}
}

// reportProgress periodically reports progress if enabled
func (c *ParallelMigrationCoordinator) reportProgress() {
	ticker := time.NewTicker(c.config.ProgressReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.logProgress()
		case <-c.progressReportCh:
			c.logProgress()
		case <-c.shutdownCh:
			return
		}
	}
}

// logProgress logs current progress statistics
func (c *ParallelMigrationCoordinator) logProgress() {
	c.resultsMu.Lock()
	defer c.resultsMu.Unlock()

	queueSize := c.jobQueue.Size()
	totalResults := len(c.results)
	successCount := 0
	
	for _, result := range c.results {
		if result.Success {
			successCount++
		}
	}

	logger.User.Infof("Migration Progress: %d completed, %d successful, %d in queue", 
		totalResults, successCount, queueSize)
}

// WaitForCompletion waits for all migration jobs to finish and returns a summary
func (c *ParallelMigrationCoordinator) WaitForCompletion(timeout time.Duration) (*MigrationSummary, error) {
	if !c.isStarted {
		return nil, fmt.Errorf("coordinator not started")
	}

	logger.Op.WithFields(map[string]interface{}{
		"timeout": timeout.String(),
	}).Info("Waiting for migration completion")

	// Set up timeout context
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	} else {
		ctx, cancel = context.WithCancel(context.Background())
		defer cancel()
	}

	// Wait for completion or timeout
	select {
	case <-c.doneCh:
		logger.Op.Info("All migrations completed successfully")
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			logger.Op.Warn("Migration completion timed out")
			return nil, fmt.Errorf("migration completion timed out after %v", timeout)
		}
		return nil, ctx.Err()
	}

	// Collect and aggregate results
	c.endTime = time.Now()
	summary := c.generateMigrationSummary()

	logger.Op.WithFields(map[string]interface{}{
		"totalJobs":      summary.TotalJobs,
		"successfulJobs": summary.SuccessfulJobs,
		"failedJobs":     summary.FailedJobsCount,
		"totalDuration":  summary.TotalDuration.String(),
	}).Info("Migration completion summary generated")

	return summary, nil
}

// generateMigrationSummary creates a comprehensive migration summary
func (c *ParallelMigrationCoordinator) generateMigrationSummary() *MigrationSummary {
	c.resultsMu.Lock()
	defer c.resultsMu.Unlock()

	summary := &MigrationSummary{
		StartTime: c.startTime,
		EndTime:   c.endTime,
		JobResults: make([]*JobResult, len(c.results)),
		FailedJobs: make([]*JobResult, 0),
		RetryableJobs: make([]*JobResult, 0),
	}

	// Copy results to avoid race conditions
	copy(summary.JobResults, c.results)
	summary.TotalJobs = len(c.results)
	summary.TotalDuration = c.endTime.Sub(c.startTime)

	// Aggregate statistics
	var totalInstanceDuration time.Duration
	for _, result := range c.results {
		if result.Success {
			summary.SuccessfulJobs++
			summary.SuccessfulInstances++
			
			if result.InstanceMigration != nil {
				summary.SuccessfulDisks += len(result.InstanceMigration.Results)
				for _, diskResult := range result.InstanceMigration.Results {
					if diskResult.Success {
						summary.SuccessfulDisks++
					} else {
						summary.FailedDisks++
					}
				}
			}
		} else {
			summary.FailedJobsCount++
			summary.FailedInstances++
			summary.FailedJobs = append(summary.FailedJobs, result)
			
			// Check if job is retryable
			if result.InstanceMigration != nil {
				// Add logic to determine if job is retryable based on error type
				summary.RetryableJobs = append(summary.RetryableJobs, result)
			}
		}

		// Calculate total instance count and disk count
		summary.TotalInstances++
		if result.InstanceMigration != nil {
			summary.TotalDisks += len(result.InstanceMigration.Disks)
		}

		// Track duration for average calculation
		totalInstanceDuration += result.ProcessedAt.Sub(c.startTime)
	}

	// Calculate completion percentage
	if summary.TotalJobs > 0 {
		summary.CompletionPercent = float64(summary.SuccessfulJobs) / float64(summary.TotalJobs) * 100
	}

	// Calculate average duration
	if summary.TotalJobs > 0 {
		summary.AverageDuration = totalInstanceDuration / time.Duration(summary.TotalJobs)
	}

	// Calculate throughput
	if summary.TotalDuration.Seconds() > 0 {
		summary.ThroughputJobsPerSecond = float64(summary.TotalJobs) / summary.TotalDuration.Seconds()
		summary.ThroughputInstancesPerSecond = float64(summary.TotalInstances) / summary.TotalDuration.Seconds()
	}

	// Set peak concurrency (simplified - would need more tracking in real implementation)
	summary.PeakConcurrency = c.config.MaxConcurrency

	return summary
}

// Shutdown gracefully terminates all components of the migration process
func (c *ParallelMigrationCoordinator) Shutdown(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isShutdown {
		return nil // Already shutdown
	}

	if !c.isStarted {
		return fmt.Errorf("coordinator not started")
	}

	logger.Op.WithFields(map[string]interface{}{
		"timeout": timeout.String(),
	}).Info("Shutting down parallel migration coordinator")

	c.isShutdown = true

	// Signal shutdown to all goroutines
	close(c.shutdownCh)

	// Create timeout context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Shutdown components in reverse order of startup
	shutdownErrors := make([]error, 0)

	// Stop accepting new jobs and wait for workers to finish
	if c.workerPool != nil && c.workerPool.IsStarted() {
		if err := c.workerPool.Shutdown(timeout / 2); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("worker pool shutdown error: %w", err))
		}
	}

	// Close the job queue
	if c.jobQueue != nil {
		c.jobQueue.Shutdown()
		
		// Drain any remaining jobs
		remainingJobs := c.jobQueue.DrainJobs()
		if len(remainingJobs) > 0 {
			logger.Op.WithFields(map[string]interface{}{
				"drainedJobs": len(remainingJobs),
			}).Warn("Drained unprocessed jobs during shutdown")
		}
	}

	// Clean up progress tracking
	if c.progressTracker != nil {
		for _, jobID := range c.progressTracker.GetActiveJobs() {
			c.progressTracker.CleanupJob(jobID)
		}
	}

	// Clean up resource locks
	if c.resourceLocker != nil {
		// Clean up any stale locks
		staleCount := c.resourceLocker.CleanupStaleLocksOlderThan(time.Hour)
		if staleCount > 0 {
			logger.Op.WithFields(map[string]interface{}{
				"staleLocksRemoved": staleCount,
			}).Info("Cleaned up stale resource locks during shutdown")
		}
	}

	// Wait for result collection to finish
	select {
	case <-c.doneCh:
		// Result collection finished normally
	case <-ctx.Done():
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown timed out waiting for result collection"))
	}

	if len(shutdownErrors) > 0 {
		logger.Op.WithFields(map[string]interface{}{
			"errorCount": len(shutdownErrors),
		}).Error("Shutdown completed with errors")
		return fmt.Errorf("shutdown completed with %d errors: %v", len(shutdownErrors), shutdownErrors)
	}

	logger.Op.Info("Parallel migration coordinator shutdown completed successfully")
	return nil
}

// GetStatus returns current status and progress information
func (c *ParallelMigrationCoordinator) GetStatus() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.resultsMu.Lock()
	defer c.resultsMu.Unlock()

	status := map[string]interface{}{
		"is_started":        c.isStarted,
		"is_shutdown":       c.isShutdown,
		"start_time":        c.startTime,
		"queue_size":        c.jobQueue.Size(),
		"worker_count":      c.workerPool.GetWorkerCount(),
		"worker_pool_started": c.workerPool.IsStarted(),
		"total_results":     len(c.results),
	}

	if c.isStarted && !c.startTime.IsZero() {
		status["elapsed_time"] = time.Since(c.startTime)
	}

	// Add result statistics
	successCount := 0
	for _, result := range c.results {
		if result.Success {
			successCount++
		}
	}
	status["successful_results"] = successCount
	status["failed_results"] = len(c.results) - successCount

	// Add queue metrics if available
	if metrics := c.jobQueue.GetMetrics(); metrics.GetJobsEnqueued() > 0 {
		status["jobs_enqueued"] = metrics.GetJobsEnqueued()
		status["jobs_dequeued"] = metrics.GetJobsDequeued()
		status["jobs_dropped"] = metrics.GetJobsDropped()
	}

	return status
}

// shutdown internal shutdown helper
func (c *ParallelMigrationCoordinator) shutdown() {
	close(c.shutdownCh)
	
	if c.workerPool != nil && c.workerPool.IsStarted() {
		c.workerPool.Shutdown(c.config.ShutdownTimeout)
	}
	
	if c.jobQueue != nil {
		c.jobQueue.Shutdown()
	}
}