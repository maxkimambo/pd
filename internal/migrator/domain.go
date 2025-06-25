package migrator

import (
	"context"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

type InstanceState int

const (
	InstanceStateUnknown InstanceState = iota
	InstanceStateRunning
	InstanceStateStopped
	InstanceStateSuspended
)

type MigrationStatus int

const (
	MigrationStatusPending MigrationStatus = iota
	MigrationStatusInProgress
	MigrationStatusCompleted
	MigrationStatusFailed
)

type InstanceMigration struct {
	Instance     *computepb.Instance
	InitialState InstanceState
	Disks        []*AttachedDiskInfo
	Results      []DiskMigrationResult
	Status       MigrationStatus
	Errors       []MigrationError
	Timing       *InstanceMigrationTiming
}

// InstanceMigrationTiming tracks detailed timing information for instance migration
type InstanceMigrationTiming struct {
	StartTime      *time.Time
	EndTime        *time.Time
	ShutdownStart  *time.Time
	ShutdownEnd    *time.Time
	StartupStart   *time.Time
	StartupEnd     *time.Time
	PhaseTiming    map[MigrationPhase]PhaseTiming
	TotalDuration  time.Duration
	DowntimeDuration time.Duration
}

// PhaseTiming tracks timing for individual migration phases
type PhaseTiming struct {
	StartTime time.Time
	EndTime   *time.Time
	Duration  time.Duration
	Success   bool
	Error     error
}

type AttachedDiskInfo struct {
	AttachedDisk *computepb.AttachedDisk
	DiskDetails  *computepb.Disk
	IsBoot       bool
}

type DiskMigrationResult struct {
	DiskInfo    *AttachedDiskInfo
	Success     bool
	NewDiskLink string
	Error       error
}

// MigrationReports is the main container for all instance migration reports
type MigrationReports struct {
	// Reports maps instance name to its individual migration report
	Reports map[string]*InstanceMigrationReport
	// Summary contains aggregated statistics across all instances
	Summary *InstanceMigrationSummary
	// SessionID for tracking this migration session
	SessionID string
	// StartTime when the entire migration session began
	StartTime time.Time
	// EndTime when the entire migration session completed
	EndTime *time.Time
}

// InstanceMigrationReport contains complete migration information for a single instance
type InstanceMigrationReport struct {
	// Instance identification
	InstanceName string
	InstanceID   string
	Zone         string
	
	// Overall timing
	StartTime        *time.Time
	EndTime          *time.Time
	TotalDuration    time.Duration
	DowntimeDuration time.Duration // Time from shutdown to startup
	
	// Status tracking
	CurrentPhase MigrationPhase
	Status       string // "pending", "running", "completed", "failed"
	
	// Phase-by-phase detailed timing
	Phases map[MigrationPhase]*PhaseReport
	
	// Disk migration results for this instance
	DiskResults []DiskMigrationResult
	
	// Any errors encountered
	Errors []error
	
	// Metadata
	InitialState InstanceState
	FinalState   InstanceState
}

// PhaseReport tracks detailed information for each migration phase
type PhaseReport struct {
	Phase     MigrationPhase
	StartTime time.Time
	EndTime   *time.Time
	Duration  time.Duration
	Status    string // "pending", "running", "completed", "failed"
	Details   string // Human-readable description of what happened
	Error     error
}

// InstanceMigrationSummary contains aggregated statistics across all instances
type InstanceMigrationSummary struct {
	TotalInstances       int
	SuccessfulInstances  int
	FailedInstances      int
	RunningInstances     int
	TotalDisks           int
	SuccessfulDisks      int
	FailedDisks          int
	AverageDowntime      time.Duration
	AverageMigrationTime time.Duration
	TotalSessionTime     time.Duration
	FastestMigration     time.Duration
	SlowestMigration     time.Duration
}

// NewMigrationReports creates a new migration reports container
func NewMigrationReports(sessionID string) *MigrationReports {
	return &MigrationReports{
		Reports:   make(map[string]*InstanceMigrationReport),
		Summary:   &InstanceMigrationSummary{},
		SessionID: sessionID,
		StartTime: time.Now(),
	}
}

// GetOrCreateInstanceReport gets an existing report or creates a new one for the instance
func (mr *MigrationReports) GetOrCreateInstanceReport(instanceName, instanceID, zone string) *InstanceMigrationReport {
	if report, exists := mr.Reports[instanceName]; exists {
		return report
	}
	
	report := &InstanceMigrationReport{
		InstanceName: instanceName,
		InstanceID:   instanceID,
		Zone:         zone,
		Status:       "pending",
		CurrentPhase: PhaseDiscovery,
		Phases:       make(map[MigrationPhase]*PhaseReport),
		DiskResults:  make([]DiskMigrationResult, 0),
		Errors:       make([]error, 0),
	}
	
	mr.Reports[instanceName] = report
	return report
}

// StartPhase marks the beginning of a migration phase for an instance
func (ir *InstanceMigrationReport) StartPhase(phase MigrationPhase, details string) {
	ir.CurrentPhase = phase
	ir.Status = "running"
	
	if ir.StartTime == nil {
		now := time.Now()
		ir.StartTime = &now
	}
	
	ir.Phases[phase] = &PhaseReport{
		Phase:     phase,
		StartTime: time.Now(),
		Status:    "running",
		Details:   details,
	}
}

// CompletePhase marks the completion of a migration phase
func (ir *InstanceMigrationReport) CompletePhase(phase MigrationPhase, success bool, err error) {
	if phaseReport, exists := ir.Phases[phase]; exists {
		now := time.Now()
		phaseReport.EndTime = &now
		phaseReport.Duration = now.Sub(phaseReport.StartTime)
		phaseReport.Error = err
		
		if success {
			phaseReport.Status = "completed"
		} else {
			phaseReport.Status = "failed"
			ir.Status = "failed"
			if err != nil {
				ir.Errors = append(ir.Errors, err)
			}
		}
	}
}

// MarkShutdownStart records when instance shutdown begins
func (ir *InstanceMigrationReport) MarkShutdownStart() {
	now := time.Now()
	if shutdownPhase, exists := ir.Phases[PhaseMigration]; exists {
		if shutdownPhase.StartTime.IsZero() {
			shutdownPhase.StartTime = now
		}
	}
}

// MarkShutdownEnd records when instance shutdown completes
func (ir *InstanceMigrationReport) MarkShutdownEnd() {
	// Shutdown completion will be tracked in the phase completion
}

// MarkStartupStart records when instance startup begins
func (ir *InstanceMigrationReport) MarkStartupStart() {
	// This would be tracked when starting the restore phase
}

// MarkStartupEnd records when instance startup completes
func (ir *InstanceMigrationReport) MarkStartupEnd() {
	// Calculate total downtime from shutdown start to startup end
	if shutdownPhase, hasShutdown := ir.Phases[PhaseMigration]; hasShutdown {
		if startupPhase, hasStartup := ir.Phases[PhaseRestore]; hasStartup {
			if startupPhase.EndTime != nil {
				ir.DowntimeDuration = startupPhase.EndTime.Sub(shutdownPhase.StartTime)
			}
		}
	}
}

// Complete marks the entire instance migration as complete
func (ir *InstanceMigrationReport) Complete(success bool) {
	now := time.Now()
	ir.EndTime = &now
	
	if ir.StartTime != nil {
		ir.TotalDuration = now.Sub(*ir.StartTime)
	}
	
	if success {
		ir.Status = "completed"
	} else {
		ir.Status = "failed"
	}
}

// UpdateSummary recalculates the summary statistics from all instance reports
func (mr *MigrationReports) UpdateSummary() {
	summary := &InstanceMigrationSummary{}
	
	var totalDowntime, totalMigrationTime time.Duration
	var migrationTimes []time.Duration
	
	for _, report := range mr.Reports {
		summary.TotalInstances++
		
		switch report.Status {
		case "completed":
			summary.SuccessfulInstances++
			totalMigrationTime += report.TotalDuration
			migrationTimes = append(migrationTimes, report.TotalDuration)
		case "failed":
			summary.FailedInstances++
		case "running":
			summary.RunningInstances++
		}
		
		summary.TotalDisks += len(report.DiskResults)
		for _, diskResult := range report.DiskResults {
			if diskResult.Success {
				summary.SuccessfulDisks++
			} else {
				summary.FailedDisks++
			}
		}
		
		if report.DowntimeDuration > 0 {
			totalDowntime += report.DowntimeDuration
		}
	}
	
	// Calculate averages
	if summary.SuccessfulInstances > 0 {
		summary.AverageMigrationTime = totalMigrationTime / time.Duration(summary.SuccessfulInstances)
		summary.AverageDowntime = totalDowntime / time.Duration(summary.SuccessfulInstances)
	}
	
	// Find fastest and slowest migrations
	if len(migrationTimes) > 0 {
		summary.FastestMigration = migrationTimes[0]
		summary.SlowestMigration = migrationTimes[0]
		for _, duration := range migrationTimes {
			if duration < summary.FastestMigration {
				summary.FastestMigration = duration
			}
			if duration > summary.SlowestMigration {
				summary.SlowestMigration = duration
			}
		}
	}
	
	// Calculate total session time
	if mr.EndTime != nil {
		summary.TotalSessionTime = mr.EndTime.Sub(mr.StartTime)
	} else {
		summary.TotalSessionTime = time.Since(mr.StartTime)
	}
	
	mr.Summary = summary
}

type InstanceMigrationService interface {
	DiscoverInstances(ctx context.Context, config *Config) ([]*InstanceMigration, error)
	MigrateInstance(ctx context.Context, migration *InstanceMigration) error
	GenerateReport(migrations []*InstanceMigration) *MigrationReports
}

type MigrationPhase int

const (
	PhaseDiscovery MigrationPhase = iota
	PhasePreparation
	PhaseMigration
	PhaseRestore
	PhaseCleanup
)

type ErrorType int

const (
	ErrorTypeTransient ErrorType = iota
	ErrorTypePermanent
	ErrorTypeUserInput
)

type MigrationError struct {
	Type   ErrorType
	Phase  MigrationPhase
	Target string
	Cause  error
}
