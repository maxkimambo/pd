package migrator

import (
	"context"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
)

// ProgressEvent represents a progress update event
type ProgressEvent struct {
	JobID           string
	InstanceID      string
	InstanceName    string
	Step            string
	Description     string
	CompletedSteps  int
	TotalSteps      int
	ProgressPercent float64
	Timestamp       time.Time
	
	// Performance metrics
	ElapsedTime         time.Duration
	EstimatedTimeLeft   time.Duration
	DisksProcessed      int
	DisksCompleted      int
	
	// Resource utilization
	MemoryUsageMB       int64
	CPUUsagePercent     float64
	NetworkBytesRead    int64
	NetworkBytesWritten int64
	
	// Additional context
	Phase               MigrationPhase
	Details             map[string]interface{}
}

// ProgressCallback is a function that receives progress updates
type ProgressCallback func(event *ProgressEvent)

// ProgressTracker manages progress tracking for migrations
type ProgressTracker struct {
	callbacks []ProgressCallback
	mutex     sync.RWMutex
	
	// Internal tracking
	events    map[string][]*ProgressEvent // jobID -> events
	metrics   map[string]*MigrationMetrics // jobID -> metrics
}

// MigrationMetrics tracks performance metrics for a migration
type MigrationMetrics struct {
	StartTime           time.Time
	LastUpdateTime      time.Time
	StepsPerSecond      float64
	AverageStepDuration time.Duration
	EstimatedCompletion time.Time
	
	// Resource usage tracking
	PeakMemoryMB        int64
	AverageCPUPercent   float64
	TotalNetworkBytes   int64
	
	// Counters
	TotalEvents         int
	ErrorEvents         int
	WarningEvents       int
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		callbacks: make([]ProgressCallback, 0),
		events:    make(map[string][]*ProgressEvent),
		metrics:   make(map[string]*MigrationMetrics),
	}
}

// AddCallback adds a progress callback
func (pt *ProgressTracker) AddCallback(callback ProgressCallback) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	pt.callbacks = append(pt.callbacks, callback)
}

// RemoveAllCallbacks removes all progress callbacks
func (pt *ProgressTracker) RemoveAllCallbacks() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	pt.callbacks = pt.callbacks[:0]
}

// ReportProgress reports a progress event
func (pt *ProgressTracker) ReportProgress(event *ProgressEvent) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	
	// Store event
	if pt.events[event.JobID] == nil {
		pt.events[event.JobID] = make([]*ProgressEvent, 0)
	}
	pt.events[event.JobID] = append(pt.events[event.JobID], event)
	
	// Update metrics
	pt.updateMetrics(event)
	
	// Notify callbacks
	for _, callback := range pt.callbacks {
		// Call callback in goroutine to avoid blocking
		go func(cb ProgressCallback, e *ProgressEvent) {
			defer func() {
				if r := recover(); r != nil {
					// Log callback panic but don't crash
					logger.Op.WithFields(map[string]interface{}{
						"jobID": e.JobID,
						"panic": r,
					}).Error("Progress callback panicked")
				}
			}()
			cb(e)
		}(callback, event)
	}
}

// updateMetrics updates internal metrics for a job
func (pt *ProgressTracker) updateMetrics(event *ProgressEvent) {
	metrics, exists := pt.metrics[event.JobID]
	if !exists {
		metrics = &MigrationMetrics{
			StartTime: event.Timestamp,
		}
		pt.metrics[event.JobID] = metrics
	}
	
	// Update basic metrics
	metrics.LastUpdateTime = event.Timestamp
	metrics.TotalEvents++
	
	// Update resource usage
	if event.MemoryUsageMB > metrics.PeakMemoryMB {
		metrics.PeakMemoryMB = event.MemoryUsageMB
	}
	
	if metrics.TotalEvents == 1 {
		metrics.AverageCPUPercent = event.CPUUsagePercent
	} else {
		// Running average
		metrics.AverageCPUPercent = (metrics.AverageCPUPercent*float64(metrics.TotalEvents-1) + event.CPUUsagePercent) / float64(metrics.TotalEvents)
	}
	
	metrics.TotalNetworkBytes = event.NetworkBytesRead + event.NetworkBytesWritten
	
	// Calculate performance metrics
	elapsedTime := event.Timestamp.Sub(metrics.StartTime)
	if elapsedTime > 0 && event.CompletedSteps > 0 {
		metrics.StepsPerSecond = float64(event.CompletedSteps) / elapsedTime.Seconds()
		metrics.AverageStepDuration = elapsedTime / time.Duration(event.CompletedSteps)
		
		// Estimate completion time
		remainingSteps := event.TotalSteps - event.CompletedSteps
		if metrics.StepsPerSecond > 0 {
			remainingTime := time.Duration(float64(remainingSteps)/metrics.StepsPerSecond) * time.Second
			metrics.EstimatedCompletion = event.Timestamp.Add(remainingTime)
		}
	}
}

// GetMetrics returns the current metrics for a job
func (pt *ProgressTracker) GetMetrics(jobID string) *MigrationMetrics {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	
	if metrics, exists := pt.metrics[jobID]; exists {
		// Return a copy to avoid race conditions
		metricsCopy := *metrics
		return &metricsCopy
	}
	return nil
}

// GetProgressHistory returns the progress history for a job
func (pt *ProgressTracker) GetProgressHistory(jobID string) []*ProgressEvent {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	
	if events, exists := pt.events[jobID]; exists {
		// Return a copy to avoid race conditions
		eventsCopy := make([]*ProgressEvent, len(events))
		copy(eventsCopy, events)
		return eventsCopy
	}
	return nil
}

// GetLatestProgress returns the latest progress event for a job
func (pt *ProgressTracker) GetLatestProgress(jobID string) *ProgressEvent {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	
	if events, exists := pt.events[jobID]; exists && len(events) > 0 {
		// Return a copy of the latest event
		latest := *events[len(events)-1]
		return &latest
	}
	return nil
}

// CleanupJob removes tracking data for a completed job
func (pt *ProgressTracker) CleanupJob(jobID string) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	
	delete(pt.events, jobID)
	delete(pt.metrics, jobID)
}

// GetActiveJobs returns a list of currently tracked job IDs
func (pt *ProgressTracker) GetActiveJobs() []string {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	
	jobs := make([]string, 0, len(pt.events))
	for jobID := range pt.events {
		jobs = append(jobs, jobID)
	}
	return jobs
}

// GetSummaryReport generates a summary report of all active jobs
func (pt *ProgressTracker) GetSummaryReport() map[string]interface{} {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()
	
	summary := map[string]interface{}{
		"total_active_jobs": len(pt.events),
		"jobs":              make(map[string]interface{}),
		"timestamp":         time.Now(),
	}
	
	jobSummaries := make(map[string]interface{})
	for jobID, events := range pt.events {
		if len(events) == 0 {
			continue
		}
		
		latest := events[len(events)-1]
		metrics := pt.metrics[jobID]
		
		jobSummary := map[string]interface{}{
			"job_id":           jobID,
			"instance_name":    latest.InstanceName,
			"current_step":     latest.Step,
			"progress_percent": latest.ProgressPercent,
			"completed_steps":  latest.CompletedSteps,
			"total_steps":      latest.TotalSteps,
			"elapsed_time":     latest.ElapsedTime,
			"phase":            latest.Phase,
		}
		
		if metrics != nil {
			jobSummary["estimated_completion"] = metrics.EstimatedCompletion
			jobSummary["steps_per_second"] = metrics.StepsPerSecond
			jobSummary["peak_memory_mb"] = metrics.PeakMemoryMB
			jobSummary["average_cpu_percent"] = metrics.AverageCPUPercent
		}
		
		jobSummaries[jobID] = jobSummary
	}
	
	summary["jobs"] = jobSummaries
	return summary
}

// ProgressReporter is an interface for reporting progress to monitoring systems
type ProgressReporter interface {
	ReportProgress(ctx context.Context, event *ProgressEvent) error
	ReportMetrics(ctx context.Context, jobID string, metrics *MigrationMetrics) error
}

// ConsoleProgressReporter reports progress to console/logs
type ConsoleProgressReporter struct{}

// NewConsoleProgressReporter creates a new console progress reporter
func NewConsoleProgressReporter() *ConsoleProgressReporter {
	return &ConsoleProgressReporter{}
}

// ReportProgress reports progress to console
func (r *ConsoleProgressReporter) ReportProgress(ctx context.Context, event *ProgressEvent) error {
	logger.User.Infof("Migration Progress [%s]: %.1f%% - %s (Step %d/%d, Elapsed: %v)", 
		event.InstanceName, 
		event.ProgressPercent, 
		event.Description,
		event.CompletedSteps,
		event.TotalSteps,
		event.ElapsedTime)
	
	return nil
}

// ReportMetrics reports metrics to console
func (r *ConsoleProgressReporter) ReportMetrics(ctx context.Context, jobID string, metrics *MigrationMetrics) error {
	logger.Op.Debugf("Migration Metrics [%s]: %.2f steps/sec, Peak Memory: %d MB, Avg CPU: %.1f%%, Events: %d", 
		jobID,
		metrics.StepsPerSecond,
		metrics.PeakMemoryMB,
		metrics.AverageCPUPercent,
		metrics.TotalEvents)
	
	return nil
}