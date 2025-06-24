package migrator

import (
	"fmt"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
)

type Config struct {
	ProjectID      string
	TargetDiskType string
	LabelFilter    string
	KmsKey         string
	KmsKeyRing     string
	KmsLocation    string
	KmsProject     string
	KmsParams      *gcp.SnapshotKmsParams
	Region         string
	Zone           string
	AutoApproveAll bool
	Concurrency    int
	RetainName     bool
	Debug          bool
	Iops           int64
	Throughput     int64
	StoragePoolId  string
	Instances      []string

	// DAG-specific configuration fields
	// MaxParallelTasks is the maximum number of tasks to run in parallel in the DAG executor
	MaxParallelTasks int `json:"max_parallel_tasks" yaml:"max_parallel_tasks"`

	// TaskTimeout is the timeout for all task-related operations including execution and dependencies
	TaskTimeout time.Duration `json:"task_timeout" yaml:"task_timeout"`

	// DAGExecutionMode determines how the DAG should be executed ("parallel" or "sequential")
	DAGExecutionMode string `json:"dag_execution_mode" yaml:"dag_execution_mode"`

	// ProgressReportInterval is how often to report progress during DAG execution
	ProgressReportInterval time.Duration `json:"progress_report_interval" yaml:"progress_report_interval"`
}

func (c *Config) PopulateKmsParams() *gcp.SnapshotKmsParams {
	if c.KmsKey != "" {
		kmsProject := c.KmsProject
		if kmsProject == "" {
			kmsProject = c.ProjectID
		}
		return &gcp.SnapshotKmsParams{
			KmsKey:      c.KmsKey,
			KmsKeyRing:  c.KmsKeyRing,
			KmsLocation: c.KmsLocation,
			KmsProject:  kmsProject,
		}
	}
	return nil
}

func (c *Config) Location() string {
	if c.Zone != "" {
		return c.Zone
	}
	return c.Region
}

// DefaultConfig returns a configuration with sensible default values
func DefaultConfig() *Config {
	return &Config{
		// Existing defaults
		ProjectID:      "",
		TargetDiskType: "pd-ssd",
		LabelFilter:    "",
		AutoApproveAll: false,
		Concurrency:    5,
		RetainName:     false,
		Debug:          false,

		// DAG-specific defaults
		MaxParallelTasks:       10,
		TaskTimeout:            15 * time.Minute,
		DAGExecutionMode:       "parallel",
		ProgressReportInterval: 30 * time.Second,
	}
}

// ApplyDefaults applies default values to any unset DAG-specific fields
// This ensures backward compatibility with existing configurations
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()

	// Apply defaults only if the field is zero-valued (unset)
	if c.MaxParallelTasks == 0 {
		c.MaxParallelTasks = defaults.MaxParallelTasks
	}
	if c.TaskTimeout == 0 {
		c.TaskTimeout = defaults.TaskTimeout
	}
	if c.DAGExecutionMode == "" {
		c.DAGExecutionMode = defaults.DAGExecutionMode
	}
	if c.ProgressReportInterval == 0 {
		c.ProgressReportInterval = defaults.ProgressReportInterval
	}

	// Ensure backward compatibility with existing Concurrency field
	if c.Concurrency == 0 {
		c.Concurrency = defaults.Concurrency
	}

	// Use existing Concurrency as MaxParallelTasks if not set explicitly
	if c.MaxParallelTasks == defaults.MaxParallelTasks && c.Concurrency != defaults.Concurrency {
		c.MaxParallelTasks = c.Concurrency
	}
}

// Validate checks if the configuration is valid and applies defaults
func (c *Config) Validate() error {
	// Apply defaults first to ensure all fields have valid values
	c.ApplyDefaults()

	// Validate required fields
	if c.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if c.TargetDiskType == "" {
		return fmt.Errorf("target disk type is required")
	}

	// Validate location
	if c.Zone == "" && c.Region == "" {
		return fmt.Errorf("either zone or region must be specified")
	}

	// Validate DAG-specific fields
	if c.MaxParallelTasks <= 0 {
		return fmt.Errorf("max_parallel_tasks must be positive, got %d", c.MaxParallelTasks)
	}
	if c.MaxParallelTasks > 100 {
		return fmt.Errorf("max_parallel_tasks too high (%d), maximum recommended is 100", c.MaxParallelTasks)
	}

	if c.TaskTimeout <= 0 {
		return fmt.Errorf("task_timeout must be positive, got %v", c.TaskTimeout)
	}

	// Validate DAG execution mode
	if c.DAGExecutionMode != "parallel" && c.DAGExecutionMode != "sequential" {
		return fmt.Errorf("dag_execution_mode must be 'parallel' or 'sequential', got '%s'", c.DAGExecutionMode)
	}

	if c.ProgressReportInterval <= 0 {
		return fmt.Errorf("progress_report_interval must be positive, got %v", c.ProgressReportInterval)
	}

	return nil
}

// GetDAGExecutorConfig returns configuration suitable for the DAG executor
func (c *Config) GetDAGExecutorConfig() map[string]interface{} {
	c.ApplyDefaults()

	return map[string]interface{}{
		"max_parallel_tasks": c.MaxParallelTasks,
		"task_timeout":       c.TaskTimeout,
		"progress_interval":  c.ProgressReportInterval,
		"execution_mode":     c.DAGExecutionMode,
	}
}

// GetMaxParallelTasks returns the maximum number of parallel tasks
func (c *Config) GetMaxParallelTasks() int {
	c.ApplyDefaults()
	return c.MaxParallelTasks
}

// IsSequentialMode returns true if DAG should execute in sequential mode
func (c *Config) IsSequentialMode() bool {
	return c.DAGExecutionMode == "sequential"
}

// IsParallelMode returns true if DAG should execute in parallel mode
func (c *Config) IsParallelMode() bool {
	return c.DAGExecutionMode == "parallel"
}
