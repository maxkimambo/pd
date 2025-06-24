package migrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/stretchr/testify/assert"
)

// Helper function for creating string pointers
func stringPtrForRetry(s string) *string {
	return &s
}

func TestNewDefaultRetryPolicy(t *testing.T) {
	policy := NewDefaultRetryPolicy()

	assert.NotNil(t, policy)
	assert.Equal(t, 3, policy.MaxRetries)
	assert.Equal(t, 5*time.Second, policy.InitialBackoff)
	assert.Equal(t, 2*time.Minute, policy.MaxBackoff)
	assert.Equal(t, 2.0, policy.BackoffFactor)
	assert.True(t, policy.EnableJitter)
	assert.Equal(t, 0.3, policy.JitterFactor)
}

func TestNewCustomRetryPolicy(t *testing.T) {
	policy := NewCustomRetryPolicy(5, 10*time.Second, 5*time.Minute, 1.5)

	assert.NotNil(t, policy)
	assert.Equal(t, 5, policy.MaxRetries)
	assert.Equal(t, 10*time.Second, policy.InitialBackoff)
	assert.Equal(t, 5*time.Minute, policy.MaxBackoff)
	assert.Equal(t, 1.5, policy.BackoffFactor)
	assert.True(t, policy.EnableJitter)
	assert.Equal(t, 0.3, policy.JitterFactor)
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	policy := NewDefaultRetryPolicy()

	tests := []struct {
		name     string
		attempts int
		expected bool
	}{
		{
			name:     "No attempts yet",
			attempts: 0,
			expected: true,
		},
		{
			name:     "One attempt",
			attempts: 1,
			expected: true,
		},
		{
			name:     "Two attempts",
			attempts: 2,
			expected: true,
		},
		{
			name:     "Three attempts (at limit)",
			attempts: 3,
			expected: false,
		},
		{
			name:     "Four attempts (over limit)",
			attempts: 4,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &MigrationJob{
				Attempts:   tt.attempts,
				MaxRetries: 3,
			}

			result := policy.ShouldRetry(job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryPolicy_GetBackoffDuration(t *testing.T) {
	policy := &RetryPolicy{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
		EnableJitter:   false, // Disable jitter for predictable testing
	}

	tests := []struct {
		name     string
		attempts int
		expected time.Duration
	}{
		{
			name:     "Zero attempts",
			attempts: 0,
			expected: 0,
		},
		{
			name:     "First attempt",
			attempts: 1,
			expected: 1 * time.Second,
		},
		{
			name:     "Second attempt",
			attempts: 2,
			expected: 2 * time.Second,
		},
		{
			name:     "Third attempt",
			attempts: 3,
			expected: 4 * time.Second,
		},
		{
			name:     "Fourth attempt",
			attempts: 4,
			expected: 8 * time.Second,
		},
		{
			name:     "Large attempt (should cap at max)",
			attempts: 10,
			expected: 30 * time.Second, // Capped at MaxBackoff
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := policy.GetBackoffDuration(tt.attempts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryPolicy_GetBackoffDuration_WithJitter(t *testing.T) {
	policy := &RetryPolicy{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
		EnableJitter:   true,
		JitterFactor:   0.3, // 30% jitter
	}

	// Run multiple times to check jitter variance
	results := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		results[i] = policy.GetBackoffDuration(2)
	}

	// Check that results are within expected range (base to base * 1.3)
	expectedMin := 2 * time.Second
	expectedMax := time.Duration(float64(2*time.Second) * 1.3)

	for _, result := range results {
		assert.GreaterOrEqual(t, result, expectedMin)
		assert.LessOrEqual(t, result, expectedMax)
	}

	// Ensure we got some variance (not all the same)
	allSame := true
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Jitter should produce different values")
}

func TestRetryPolicy_IsRetryableError(t *testing.T) {
	policy := NewDefaultRetryPolicy()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "Retryable network error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "Retryable temporary error",
			err:      errors.New("temporary failure"),
			expected: true,
		},
		{
			name:     "Non-retryable not found error",
			err:      errors.New("instance not found"),
			expected: false,
		},
		{
			name:     "Non-retryable permission error",
			err:      errors.New("permission denied"),
			expected: false,
		},
		{
			name:     "Non-retryable quota error",
			err:      errors.New("quota exceeded"),
			expected: false,
		},
		{
			name:     "Non-retryable configuration error",
			err:      errors.New("invalid configuration"),
			expected: false,
		},
		{
			name:     "Non-retryable duplicate error",
			err:      errors.New("disk already exists"),
			expected: false,
		},
		{
			name:     "Case insensitive matching",
			err:      errors.New("Instance Not Found"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := policy.IsRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDeadLetterQueue(t *testing.T) {
	t.Run("Default size", func(t *testing.T) {
		queue := NewDeadLetterQueue()
		assert.NotNil(t, queue)
		assert.Equal(t, 1000, queue.maxSize)
		assert.Empty(t, queue.failedJobs)
	})

	t.Run("Custom size", func(t *testing.T) {
		queue := NewDeadLetterQueue(500)
		assert.NotNil(t, queue)
		assert.Equal(t, 500, queue.maxSize)
		assert.Empty(t, queue.failedJobs)
	})

	t.Run("Zero size uses default", func(t *testing.T) {
		queue := NewDeadLetterQueue(0)
		assert.NotNil(t, queue)
		assert.Equal(t, 1000, queue.maxSize)
	})
}

func TestDeadLetterQueue_AddFailedJob(t *testing.T) {
	queue := NewDeadLetterQueue(3) // Small size for testing
	policy := NewDefaultRetryPolicy()

	job1 := &MigrationJob{
		ID:       "job-1",
		Instance: &computepb.Instance{Name: stringPtrForRetry("instance-1")},
		Attempts: 3,
	}

	job2 := &MigrationJob{
		ID:       "job-2",
		Instance: &computepb.Instance{Name: stringPtrForRetry("instance-2")},
		Attempts: 2,
	}

	err1 := errors.New("network timeout")
	err2 := errors.New("permission denied")

	// Add first failed job
	queue.AddFailedJob(job1, err1, time.Minute, policy)

	assert.Equal(t, 1, queue.GetFailedJobCount())
	failedJobs := queue.GetFailedJobs()
	assert.Len(t, failedJobs, 1)
	assert.Equal(t, "job-1", failedJobs[0].Job.ID)
	assert.Equal(t, "network timeout", failedJobs[0].ErrorMessage)
	assert.True(t, failedJobs[0].IsRetryable)
	assert.Equal(t, 3, failedJobs[0].TotalAttempts)

	// Add second failed job
	queue.AddFailedJob(job2, err2, 30*time.Second, policy)

	assert.Equal(t, 2, queue.GetFailedJobCount())
	failedJobs = queue.GetFailedJobs()
	assert.Len(t, failedJobs, 2)
	assert.Equal(t, "job-2", failedJobs[1].Job.ID)
	assert.Equal(t, "permission denied", failedJobs[1].ErrorMessage)
	assert.False(t, failedJobs[1].IsRetryable)
}

func TestDeadLetterQueue_SizeLimit(t *testing.T) {
	queue := NewDeadLetterQueue(2) // Very small size
	policy := NewDefaultRetryPolicy()

	// Add 3 jobs to exceed the limit
	for i := 1; i <= 3; i++ {
		job := &MigrationJob{
			ID:       fmt.Sprintf("job-%d", i),
			Instance: &computepb.Instance{Name: stringPtrForRetry(fmt.Sprintf("instance-%d", i))},
			Attempts: 1,
		}
		queue.AddFailedJob(job, errors.New("test error"), time.Second, policy)
	}

	// Should only have 2 jobs (FIFO eviction)
	assert.Equal(t, 2, queue.GetFailedJobCount())
	failedJobs := queue.GetFailedJobs()

	// Should have job-2 and job-3 (job-1 evicted)
	assert.Equal(t, "job-2", failedJobs[0].Job.ID)
	assert.Equal(t, "job-3", failedJobs[1].Job.ID)
}

func TestDeadLetterQueue_GetRetryableJobs(t *testing.T) {
	queue := NewDeadLetterQueue()
	policy := NewDefaultRetryPolicy()

	// Add retryable job
	retryableJob := &MigrationJob{
		ID:       "retryable-job",
		Instance: &computepb.Instance{Name: stringPtrForRetry("retryable-instance")},
		Attempts: 2,
	}
	queue.AddFailedJob(retryableJob, errors.New("timeout"), time.Second, policy)

	// Add non-retryable job
	nonRetryableJob := &MigrationJob{
		ID:       "non-retryable-job",
		Instance: &computepb.Instance{Name: stringPtrForRetry("non-retryable-instance")},
		Attempts: 3,
	}
	queue.AddFailedJob(nonRetryableJob, errors.New("not found"), time.Second, policy)

	retryableJobs := queue.GetRetryableJobs()
	assert.Len(t, retryableJobs, 1)
	assert.Equal(t, "retryable-job", retryableJobs[0].Job.ID)
}

func TestDeadLetterQueue_Clear(t *testing.T) {
	queue := NewDeadLetterQueue()
	policy := NewDefaultRetryPolicy()

	// Add some jobs
	for i := 1; i <= 3; i++ {
		job := &MigrationJob{
			ID:       fmt.Sprintf("job-%d", i),
			Instance: &computepb.Instance{Name: stringPtrForRetry(fmt.Sprintf("instance-%d", i))},
			Attempts: 1,
		}
		queue.AddFailedJob(job, errors.New("test error"), time.Second, policy)
	}

	assert.Equal(t, 3, queue.GetFailedJobCount())

	queue.Clear()

	assert.Equal(t, 0, queue.GetFailedJobCount())
	assert.Empty(t, queue.GetFailedJobs())
}

func TestNewRetryManager(t *testing.T) {
	t.Run("With provided policy and queue", func(t *testing.T) {
		policy := NewCustomRetryPolicy(5, time.Second, time.Minute, 1.5)
		queue := NewDeadLetterQueue(100)

		manager := NewRetryManager(policy, queue)

		assert.NotNil(t, manager)
		assert.Equal(t, policy, manager.retryPolicy)
		assert.Equal(t, queue, manager.deadLetterQueue)
	})

	t.Run("With nil policy and queue", func(t *testing.T) {
		manager := NewRetryManager(nil, nil)

		assert.NotNil(t, manager)
		assert.NotNil(t, manager.retryPolicy)
		assert.NotNil(t, manager.deadLetterQueue)
		assert.Equal(t, 3, manager.retryPolicy.MaxRetries) // Default policy
	})
}

func TestRetryManager_ProcessJobWithRetry(t *testing.T) {
	t.Run("Job succeeds on first attempt", func(t *testing.T) {
		manager := NewRetryManager(NewDefaultRetryPolicy(), NewDeadLetterQueue())

		job := &MigrationJob{
			ID:         "test-job",
			Instance:   &computepb.Instance{Name: stringPtrForRetry("test-instance")},
			Attempts:   0,
			MaxRetries: 3,
		}

		processor := func(ctx context.Context, job *MigrationJob) error {
			return nil // Success
		}

		err := manager.ProcessJobWithRetry(context.Background(), job, processor)

		assert.NoError(t, err)
		assert.Equal(t, 1, job.Attempts)
		assert.Equal(t, 0, manager.deadLetterQueue.GetFailedJobCount())
	})

	t.Run("Job succeeds after retry", func(t *testing.T) {
		manager := NewRetryManager(
			&RetryPolicy{
				MaxRetries:     3,
				InitialBackoff: 1 * time.Millisecond, // Very short for testing
				MaxBackoff:     10 * time.Millisecond,
				BackoffFactor:  2.0,
				EnableJitter:   false,
			},
			NewDeadLetterQueue(),
		)

		job := &MigrationJob{
			ID:         "test-job",
			Instance:   &computepb.Instance{Name: stringPtrForRetry("test-instance")},
			Attempts:   0,
			MaxRetries: 3,
		}

		callCount := 0
		processor := func(ctx context.Context, job *MigrationJob) error {
			callCount++
			if callCount < 3 {
				return errors.New("temporary failure")
			}
			return nil // Success on third call
		}

		err := manager.ProcessJobWithRetry(context.Background(), job, processor)

		assert.NoError(t, err)
		assert.Equal(t, 3, job.Attempts)
		assert.Equal(t, 3, callCount)
		assert.Equal(t, 0, manager.deadLetterQueue.GetFailedJobCount())
	})

	t.Run("Job fails permanently after retries", func(t *testing.T) {
		manager := NewRetryManager(
			&RetryPolicy{
				MaxRetries:     2,
				InitialBackoff: 1 * time.Millisecond,
				MaxBackoff:     10 * time.Millisecond,
				BackoffFactor:  2.0,
				EnableJitter:   false,
			},
			NewDeadLetterQueue(),
		)

		job := &MigrationJob{
			ID:         "test-job",
			Instance:   &computepb.Instance{Name: stringPtrForRetry("test-instance")},
			Attempts:   0,
			MaxRetries: 2,
		}

		processor := func(ctx context.Context, job *MigrationJob) error {
			return errors.New("persistent failure")
		}

		err := manager.ProcessJobWithRetry(context.Background(), job, processor)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "job permanently failed after 2 attempts")
		assert.Equal(t, 2, job.Attempts)
		assert.Equal(t, 1, manager.deadLetterQueue.GetFailedJobCount())
	})

	t.Run("Context cancellation", func(t *testing.T) {
		manager := NewRetryManager(NewDefaultRetryPolicy(), NewDeadLetterQueue())

		job := &MigrationJob{
			ID:         "test-job",
			Instance:   &computepb.Instance{Name: stringPtrForRetry("test-instance")},
			Attempts:   0,
			MaxRetries: 3,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		processor := func(ctx context.Context, job *MigrationJob) error {
			return errors.New("should not be called")
		}

		err := manager.ProcessJobWithRetry(ctx, job, processor)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		assert.Equal(t, 0, job.Attempts) // Should not increment if context is cancelled before processing
	})
}

func TestRetryManager_GetRetryStats(t *testing.T) {
	manager := NewRetryManager(NewDefaultRetryPolicy(), NewDeadLetterQueue())

	stats := manager.GetRetryStats()

	assert.Contains(t, stats, "total_retries")
	assert.Contains(t, stats, "successful_retries")
	assert.Contains(t, stats, "failed_jobs_count")
	assert.Contains(t, stats, "retry_policy")

	assert.Equal(t, int64(0), stats["total_retries"])
	assert.Equal(t, int64(0), stats["successful_retries"])
	assert.Equal(t, 0, stats["failed_jobs_count"])
}
