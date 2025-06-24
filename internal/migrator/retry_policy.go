package migrator

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
)

// RetryPolicy defines the retry configuration for failed migration jobs
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	BackoffFactor  float64       `json:"backoff_factor"`
	EnableJitter   bool          `json:"enable_jitter"`
	JitterFactor   float64       `json:"jitter_factor"` // Percentage of jitter (0.0 to 1.0)
}

// NewDefaultRetryPolicy creates a retry policy with sensible defaults
func NewDefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     2 * time.Minute,
		BackoffFactor:  2.0,
		EnableJitter:   true,
		JitterFactor:   0.3, // 30% jitter
	}
}

// NewCustomRetryPolicy creates a retry policy with custom settings
func NewCustomRetryPolicy(maxRetries int, initialBackoff, maxBackoff time.Duration, backoffFactor float64) *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     maxRetries,
		InitialBackoff: initialBackoff,
		MaxBackoff:     maxBackoff,
		BackoffFactor:  backoffFactor,
		EnableJitter:   true,
		JitterFactor:   0.3,
	}
}

// ShouldRetry returns true if the job should be retried based on attempt count
func (p *RetryPolicy) ShouldRetry(job *MigrationJob) bool {
	return job.Attempts < p.MaxRetries
}

// GetBackoffDuration calculates the backoff duration for a given attempt count
func (p *RetryPolicy) GetBackoffDuration(attempts int) time.Duration {
	if attempts <= 0 {
		return 0
	}

	// Calculate exponential backoff: initialBackoff * (factor ^ (attempts-1))
	backoff := time.Duration(float64(p.InitialBackoff) * math.Pow(p.BackoffFactor, float64(attempts-1)))

	// Cap at maximum backoff
	if backoff > p.MaxBackoff {
		backoff = p.MaxBackoff
	}

	// Add jitter to prevent thundering herd problem
	if p.EnableJitter {
		jitter := rand.Float64() * p.JitterFactor
		backoff = time.Duration(float64(backoff) * (1 + jitter))
	}

	return backoff
}

// IsRetryableError determines if an error is worth retrying
func (p *RetryPolicy) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Add logic to determine if error is retryable
	// For now, consider most errors retryable except for specific permanent failures
	errorMsg := err.Error()

	// Non-retryable errors (permanent failures)
	permanentErrors := []string{
		"not found",
		"permission denied",
		"quota exceeded",
		"invalid configuration",
		"disk already exists",
		"instance not found",
	}

	for _, permanentErr := range permanentErrors {
		if containsIgnoreCase(errorMsg, permanentErr) {
			return false
		}
	}

	// Most other errors are considered retryable (network issues, timeouts, etc.)
	return true
}

// containsIgnoreCase checks if a string contains a substring (case insensitive)
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive search without importing strings package
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1, c2 := s[i+j], substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// DeadLetterQueue stores jobs that have permanently failed after exhausting all retries
type DeadLetterQueue struct {
	failedJobs []*FailedJob
	mu         sync.RWMutex
	maxSize    int
}

// FailedJob represents a job that has permanently failed
type FailedJob struct {
	Job           *MigrationJob `json:"job"`
	LastError     error         `json:"last_error,omitempty"`
	ErrorMessage  string        `json:"error_message"`
	FailedAt      time.Time     `json:"failed_at"`
	TotalAttempts int           `json:"total_attempts"`
	TotalDuration time.Duration `json:"total_duration"`
	IsRetryable   bool          `json:"is_retryable"`
}

// NewDeadLetterQueue creates a new dead letter queue with optional max size
func NewDeadLetterQueue(maxSize ...int) *DeadLetterQueue {
	size := 1000 // Default max size
	if len(maxSize) > 0 && maxSize[0] > 0 {
		size = maxSize[0]
	}

	return &DeadLetterQueue{
		failedJobs: make([]*FailedJob, 0),
		maxSize:    size,
	}
}

// AddFailedJob adds a permanently failed job to the dead letter queue
func (q *DeadLetterQueue) AddFailedJob(job *MigrationJob, err error, totalDuration time.Duration, retryPolicy *RetryPolicy) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Create failed job entry
	failedJob := &FailedJob{
		Job:           job,
		LastError:     err,
		ErrorMessage:  "",
		FailedAt:      time.Now(),
		TotalAttempts: job.Attempts,
		TotalDuration: totalDuration,
		IsRetryable:   false,
	}

	if err != nil {
		failedJob.ErrorMessage = err.Error()
		if retryPolicy != nil {
			failedJob.IsRetryable = retryPolicy.IsRetryableError(err)
		}
	}

	// Add to queue with size limit
	q.failedJobs = append(q.failedJobs, failedJob)

	// Trim if exceeding max size (FIFO)
	if len(q.failedJobs) > q.maxSize {
		q.failedJobs = q.failedJobs[1:]
	}

	// Safely log with nil check for testing environments
	if logger.Op != nil {
		logger.Op.WithFields(map[string]interface{}{
			"jobID":         job.ID,
			"instanceName":  job.GetInstanceName(),
			"totalAttempts": job.Attempts,
			"errorMessage":  failedJob.ErrorMessage,
			"isRetryable":   failedJob.IsRetryable,
		}).Warn("Job permanently failed and added to dead letter queue")
	}
}

// GetFailedJobs returns a copy of all failed jobs
func (q *DeadLetterQueue) GetFailedJobs() []*FailedJob {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Return a copy to prevent external modification
	jobs := make([]*FailedJob, len(q.failedJobs))
	copy(jobs, q.failedJobs)

	return jobs
}

// GetRetryableJobs returns failed jobs that could potentially be retried
func (q *DeadLetterQueue) GetRetryableJobs() []*FailedJob {
	q.mu.RLock()
	defer q.mu.RUnlock()

	retryableJobs := make([]*FailedJob, 0)
	for _, job := range q.failedJobs {
		if job.IsRetryable {
			retryableJobs = append(retryableJobs, job)
		}
	}

	return retryableJobs
}

// GetFailedJobCount returns the number of failed jobs in the queue
func (q *DeadLetterQueue) GetFailedJobCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.failedJobs)
}

// Clear removes all failed jobs from the queue
func (q *DeadLetterQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := len(q.failedJobs)
	q.failedJobs = make([]*FailedJob, 0)

	if logger.Op != nil {
		logger.Op.WithFields(map[string]interface{}{
			"clearedJobs": count,
		}).Info("Cleared dead letter queue")
	}
}

// RetryManager coordinates the retry logic for migration jobs
type RetryManager struct {
	retryPolicy     *RetryPolicy
	deadLetterQueue *DeadLetterQueue

	// Statistics
	totalRetries      int64
	successfulRetries int64
	mu                sync.RWMutex
}

// NewRetryManager creates a new retry manager with the given policy
func NewRetryManager(retryPolicy *RetryPolicy, deadLetterQueue *DeadLetterQueue) *RetryManager {
	if retryPolicy == nil {
		retryPolicy = NewDefaultRetryPolicy()
	}
	if deadLetterQueue == nil {
		deadLetterQueue = NewDeadLetterQueue()
	}

	return &RetryManager{
		retryPolicy:     retryPolicy,
		deadLetterQueue: deadLetterQueue,
	}
}

// ProcessJobWithRetry handles the retry logic for a single job
func (rm *RetryManager) ProcessJobWithRetry(
	ctx context.Context,
	job *MigrationJob,
	processor func(context.Context, *MigrationJob) error,
) error {
	startTime := time.Now()
	var lastErr error

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Attempt to process the job
		job.Attempts++

		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"jobID":        job.ID,
				"instanceName": job.GetInstanceName(),
				"attempt":      job.Attempts,
				"maxRetries":   job.MaxRetries,
			}).Debug("Processing job attempt")
		}

		lastErr = processor(ctx, job)

		if lastErr == nil {
			// Job succeeded
			if job.Attempts > 1 {
				rm.mu.Lock()
				rm.successfulRetries++
				rm.mu.Unlock()

				if logger.Op != nil {
					logger.Op.WithFields(map[string]interface{}{
						"jobID":        job.ID,
						"instanceName": job.GetInstanceName(),
						"attempts":     job.Attempts,
					}).Info("Job succeeded after retry")
				}
			}
			return nil
		}

		// Job failed, check if we should retry
		if !rm.retryPolicy.ShouldRetry(job) || !rm.retryPolicy.IsRetryableError(lastErr) {
			// Job permanently failed
			totalDuration := time.Since(startTime)
			rm.deadLetterQueue.AddFailedJob(job, lastErr, totalDuration, rm.retryPolicy)

			if logger.Op != nil {
				logger.Op.WithFields(map[string]interface{}{
					"jobID":        job.ID,
					"instanceName": job.GetInstanceName(),
					"attempts":     job.Attempts,
					"error":        lastErr.Error(),
				}).Error("Job permanently failed after exhausting retries")
			}

			return fmt.Errorf("job permanently failed after %d attempts: %w", job.Attempts, lastErr)
		}

		// Calculate backoff and wait
		backoff := rm.retryPolicy.GetBackoffDuration(job.Attempts)

		rm.mu.Lock()
		rm.totalRetries++
		rm.mu.Unlock()

		if logger.Op != nil {
			logger.Op.WithFields(map[string]interface{}{
				"jobID":        job.ID,
				"instanceName": job.GetInstanceName(),
				"attempt":      job.Attempts,
				"backoff":      backoff.String(),
				"error":        lastErr.Error(),
			}).Warn("Job failed, retrying after backoff")
		}

		// Wait for backoff period
		select {
		case <-time.After(backoff):
			// Continue to retry
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// GetRetryStats returns retry statistics
func (rm *RetryManager) GetRetryStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"total_retries":      rm.totalRetries,
		"successful_retries": rm.successfulRetries,
		"failed_jobs_count":  rm.deadLetterQueue.GetFailedJobCount(),
		"retry_policy":       rm.retryPolicy,
	}
}
