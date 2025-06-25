package progress

import (
	"fmt"
	"strings"
	"time"
)

// Phase represents a phase of the migration process
type Phase string

const (
	PhaseDiscovery   Phase = "Discovery"
	PhaseValidation  Phase = "Validation"
	PhaseSnapshot    Phase = "Snapshot"
	PhaseShutdown    Phase = "Shutdown"
	PhaseDetach      Phase = "Detach"
	PhaseMigration   Phase = "Migration"
	PhaseAttach      Phase = "Attach"
	PhaseStartup     Phase = "Startup"
	PhaseCleanup     Phase = "Cleanup"
	PhaseCompletion  Phase = "Completion"
)

// TaskType represents the type of task being executed
type TaskType string

const (
	TaskTypeSnapshot      TaskType = "Snapshot"
	TaskTypeInstanceState TaskType = "InstanceState"
	TaskTypeDiskAttachment TaskType = "DiskAttachment"
	TaskTypeDiskMigration TaskType = "DiskMigration"
	TaskTypeCleanup       TaskType = "Cleanup"
)

// ProgressInfo contains detailed progress information
type ProgressInfo struct {
	CurrentPhase     Phase
	TotalTasks       int
	CompletedTasks   int
	FailedTasks      int
	RunningTasks     int
	ElapsedTime      time.Duration
	EstimatedTimeLeft time.Duration
	TaskBreakdown    map[TaskType]TaskStats
	CurrentOperation string
	InstanceStats    map[string]InstanceProgress
}

// TaskStats provides statistics for each task type
type TaskStats struct {
	Total       int
	Completed   int
	Failed      int
	Running     int
	Pending     int
	RunningTasks []string  // Names of currently running tasks
}

// InstanceProgress tracks progress per instance
type InstanceProgress struct {
	InstanceName    string
	CurrentPhase    Phase
	DisksProcessed  int
	TotalDisks      int
	Status          string
	StartTime       *time.Time
	CurrentDuration time.Duration
	EstimatedTotal  time.Duration
}

// Reporter handles enhanced progress reporting
type Reporter struct {
	startTime     time.Time
	lastReportTime time.Time
	reportInterval time.Duration
}

// NewReporter creates a new progress reporter
func NewReporter() *Reporter {
	return &Reporter{
		startTime:      time.Now(),
		lastReportTime: time.Now(),
		reportInterval: 5 * time.Second,
	}
}

// ShouldReport returns true if it's time to report progress
func (r *Reporter) ShouldReport() bool {
	return time.Since(r.lastReportTime) >= r.reportInterval
}

// Report generates a formatted progress report
func (r *Reporter) Report(info ProgressInfo) string {
	r.lastReportTime = time.Now()
	
	var sb strings.Builder
	
	// Overall progress header
	percentage := 0.0
	if info.TotalTasks > 0 {
		percentage = float64(info.CompletedTasks) / float64(info.TotalTasks) * 100
	}
	
	sb.WriteString(fmt.Sprintf("Progress: %d/%d tasks completed (%.1f%%)",
		info.CompletedTasks, info.TotalTasks, percentage))
	
	// Phase information
	if info.CurrentPhase != "" {
		sb.WriteString(fmt.Sprintf(" | Phase: %s", info.CurrentPhase))
	}
	
	// Time information
	sb.WriteString(fmt.Sprintf(" | Elapsed: %v", info.ElapsedTime.Round(time.Second)))
	
	// Current operation
	if info.CurrentOperation != "" {
		sb.WriteString(fmt.Sprintf("\n   Current: %s", info.CurrentOperation))
	}
	
	// Task breakdown
	if len(info.TaskBreakdown) > 0 {
		sb.WriteString("\n   Task Status:")
		for taskType, stats := range info.TaskBreakdown {
			if stats.Total > 0 {
				sb.WriteString(fmt.Sprintf("\n      %s: %d/%d completed",
					taskType, stats.Completed, stats.Total))
				if stats.Failed > 0 {
					sb.WriteString(fmt.Sprintf(", %d failed", stats.Failed))
				}
				if stats.Running > 0 {
					sb.WriteString(fmt.Sprintf(", %d running", stats.Running))
					// Show running task names
					if len(stats.RunningTasks) > 0 {
						sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(stats.RunningTasks, ", ")))
					}
				}
				if stats.Pending > 0 {
					sb.WriteString(fmt.Sprintf(", %d pending", stats.Pending))
				}
			}
		}
	}
	
	// Instance progress
	if len(info.InstanceStats) > 0 {
		sb.WriteString("\n   Instance Status:")
		for _, instanceProg := range info.InstanceStats {
			diskProgress := ""
			if instanceProg.TotalDisks > 0 {
				diskProgress = fmt.Sprintf(" (%d/%d disks)", 
					instanceProg.DisksProcessed, instanceProg.TotalDisks)
			}
			
			// Add timing information for running instances
			timingInfo := ""
			if instanceProg.Status == "running" && instanceProg.CurrentDuration > 0 {
				timingInfo = fmt.Sprintf(" [%s]", FormatDuration(instanceProg.CurrentDuration))
			}
			
			sb.WriteString(fmt.Sprintf("\n      %s: %s%s - %s%s",
				instanceProg.InstanceName, instanceProg.CurrentPhase, 
				diskProgress, instanceProg.Status, timingInfo))
		}
	}
	
	return sb.String()
}

// ReportPhaseStart reports the beginning of a new phase
func (r *Reporter) ReportPhaseStart(phase Phase, description string) string {
	return fmt.Sprintf("Starting %s phase: %s", phase, description)
}

// ReportPhaseComplete reports completion of a phase
func (r *Reporter) ReportPhaseComplete(phase Phase, duration time.Duration, success bool) string {
	status := "COMPLETED"
	if !success {
		status = "FAILED"
	}
	return fmt.Sprintf("%s phase %s in %v", phase, status, duration.Round(time.Second))
}

// ReportTaskStart reports the start of a task
func (r *Reporter) ReportTaskStart(taskType TaskType, description string) string {
	return fmt.Sprintf("  Starting %s: %s", taskType, description)
}

// ReportTaskComplete reports task completion
func (r *Reporter) ReportTaskComplete(taskType TaskType, description string, duration time.Duration, success bool) string {
	status := "COMPLETED"
	if !success {
		status = "FAILED"
	}
	return fmt.Sprintf("  %s %s: %s (took %v)", 
		status, taskType, description, duration.Round(time.Second))
}

// ReportError formats an error for display
func (r *Reporter) ReportError(phase Phase, operation string, err error) string {
	return fmt.Sprintf("ERROR in %s phase during %s: %v", phase, operation, err)
}

// CalculateETA estimates time remaining based on current progress
func CalculateETA(completed, total int, elapsed time.Duration) time.Duration {
	if completed <= 0 || total <= 0 || completed >= total {
		return 0
	}
	
	averageTimePerTask := elapsed / time.Duration(completed)
	remainingTasks := total - completed
	return averageTimePerTask * time.Duration(remainingTasks)
}

// FormatDuration formats a duration in a user-friendly way
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}