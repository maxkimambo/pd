package migrator

import (
	"context"

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

type MigrationReport struct {
	TotalInstances      int
	SuccessfulInstances int
	FailedInstances     int
	TotalDisks          int
	SuccessfulDisks     int
	FailedDisks         int
	InstanceResults     []*InstanceMigration
}

type InstanceMigrationService interface {
	DiscoverInstances(ctx context.Context, config *Config) ([]*InstanceMigration, error)
	MigrateInstance(ctx context.Context, migration *InstanceMigration) error
	GenerateReport(migrations []*InstanceMigration) *MigrationReport
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
