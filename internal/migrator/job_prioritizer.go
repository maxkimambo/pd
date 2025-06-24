package migrator

import (
	"strings"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// JobPrioritizer defines the interface for assigning priorities to migration jobs
type JobPrioritizer interface {
	// AssignPriority determines the priority for a given instance
	AssignPriority(instance *computepb.Instance, disks []*AttachedDiskInfo) JobPriority
	
	// Compare compares two jobs and returns:
	// -1 if job1 has higher priority than job2
	//  0 if they have equal priority
	//  1 if job2 has higher priority than job1
	Compare(job1, job2 *MigrationJob) int
}

// DefaultJobPrioritizer provides a comprehensive prioritization strategy
type DefaultJobPrioritizer struct {
	// Configuration for prioritization rules
	considerMachineType bool
	considerDiskCount   bool
	considerDiskSize    bool
	considerLabels      bool
	
	// Priority labels that indicate high priority instances
	highPriorityLabels   map[string]string
	mediumPriorityLabels map[string]string
}

// NewDefaultJobPrioritizer creates a new default job prioritizer with standard configuration
func NewDefaultJobPrioritizer() *DefaultJobPrioritizer {
	return &DefaultJobPrioritizer{
		considerMachineType: true,
		considerDiskCount:   true,
		considerDiskSize:    true,
		considerLabels:      true,
		highPriorityLabels: map[string]string{
			"environment": "production",
			"criticality": "high",
			"priority":    "high",
		},
		mediumPriorityLabels: map[string]string{
			"environment": "staging",
			"criticality": "medium",
			"priority":    "medium",
		},
	}
}

// NewCustomJobPrioritizer creates a job prioritizer with custom configuration
func NewCustomJobPrioritizer(
	considerMachineType, considerDiskCount, considerDiskSize, considerLabels bool,
	highPriorityLabels, mediumPriorityLabels map[string]string,
) *DefaultJobPrioritizer {
	if highPriorityLabels == nil {
		highPriorityLabels = make(map[string]string)
	}
	if mediumPriorityLabels == nil {
		mediumPriorityLabels = make(map[string]string)
	}
	
	return &DefaultJobPrioritizer{
		considerMachineType:  considerMachineType,
		considerDiskCount:    considerDiskCount,
		considerDiskSize:     considerDiskSize,
		considerLabels:       considerLabels,
		highPriorityLabels:   highPriorityLabels,
		mediumPriorityLabels: mediumPriorityLabels,
	}
}

// AssignPriority determines the priority for a given instance based on multiple factors
func (p *DefaultJobPrioritizer) AssignPriority(instance *computepb.Instance, disks []*AttachedDiskInfo) JobPriority {
	priority := LowPriority
	
	// Check labels first (highest precedence)
	if p.considerLabels && instance.GetLabels() != nil {
		labelPriority := p.getPriorityFromLabels(instance.GetLabels())
		if labelPriority > priority {
			priority = labelPriority
		}
	}
	
	// Consider machine type
	if p.considerMachineType {
		machineTypePriority := p.getPriorityFromMachineType(instance.GetMachineType())
		if machineTypePriority > priority {
			priority = machineTypePriority
		}
	}
	
	// Consider disk count
	if p.considerDiskCount {
		diskCountPriority := p.getPriorityFromDiskCount(len(disks))
		if diskCountPriority > priority {
			priority = diskCountPriority
		}
	}
	
	// Consider total disk size
	if p.considerDiskSize {
		diskSizePriority := p.getPriorityFromDiskSize(disks)
		if diskSizePriority > priority {
			priority = diskSizePriority
		}
	}
	
	return priority
}

// getPriorityFromLabels determines priority based on instance labels
func (p *DefaultJobPrioritizer) getPriorityFromLabels(labels map[string]string) JobPriority {
	// Check for high priority labels
	for key, value := range p.highPriorityLabels {
		if instanceValue, exists := labels[key]; exists && instanceValue == value {
			return HighPriority
		}
	}
	
	// Check for medium priority labels
	for key, value := range p.mediumPriorityLabels {
		if instanceValue, exists := labels[key]; exists && instanceValue == value {
			return MediumPriority
		}
	}
	
	return LowPriority
}

// getPriorityFromMachineType determines priority based on machine type
func (p *DefaultJobPrioritizer) getPriorityFromMachineType(machineType string) JobPriority {
	// If no machine type is provided, return low priority
	if machineType == "" {
		return LowPriority
	}
	
	// Extract machine type name from full URL if necessary
	typeName := strings.ToLower(extractMachineTypeName(machineType))
	
	// If extraction failed, return low priority
	if typeName == "" {
		return LowPriority
	}
	
	// High priority: Small instances (migrate quickly)
	if strings.Contains(typeName, "micro") || strings.Contains(typeName, "small") {
		return HighPriority
	}
	
	// Medium priority: Standard instances
	if strings.Contains(typeName, "standard") || strings.Contains(typeName, "medium") {
		return MediumPriority
	}
	
	// Low priority: Large instances (take longer, less urgent)
	if strings.Contains(typeName, "large") || strings.Contains(typeName, "xlarge") || 
	   strings.Contains(typeName, "highmem") || strings.Contains(typeName, "highcpu") {
		return LowPriority
	}
	
	// Default to medium priority for unknown types
	return MediumPriority
}

// getPriorityFromDiskCount determines priority based on number of attached disks
func (p *DefaultJobPrioritizer) getPriorityFromDiskCount(diskCount int) JobPriority {
	switch {
	case diskCount >= 5:
		return HighPriority // Many disks = more complex, handle first
	case diskCount >= 2:
		return MediumPriority
	default:
		return LowPriority // Single disk instances are simpler
	}
}

// getPriorityFromDiskSize determines priority based on total size of all disks
func (p *DefaultJobPrioritizer) getPriorityFromDiskSize(disks []*AttachedDiskInfo) JobPriority {
	var totalSize int64
	
	for _, disk := range disks {
		if disk.DiskDetails != nil && disk.DiskDetails.SizeGb != nil {
			totalSize += *disk.DiskDetails.SizeGb
		}
	}
	
	// Convert GB to TB for easier threshold checking
	totalSizeTB := float64(totalSize) / 1024.0
	
	switch {
	case totalSizeTB >= 10.0:
		return LowPriority // Very large disks take longer, lower priority
	case totalSizeTB >= 1.0:
		return MediumPriority
	default:
		return HighPriority // Small disks are quick to migrate
	}
}

// extractMachineTypeName extracts the machine type name from a full GCP URL
func extractMachineTypeName(machineType string) string {
	// Handle both full URLs and simple names
	if strings.Contains(machineType, "/") {
		parts := strings.Split(machineType, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return machineType
}

// Compare compares two migration jobs based on priority and creation time
func (p *DefaultJobPrioritizer) Compare(job1, job2 *MigrationJob) int {
	// Primary comparison: priority (higher priority first)
	if job1.Priority > job2.Priority {
		return -1
	} else if job1.Priority < job2.Priority {
		return 1
	}
	
	// Secondary comparison: creation time (earlier jobs first for same priority)
	if job1.CreatedAt.Before(job2.CreatedAt) {
		return -1
	} else if job1.CreatedAt.After(job2.CreatedAt) {
		return 1
	}
	
	// Tertiary comparison: instance name for deterministic ordering
	if job1.GetInstanceName() < job2.GetInstanceName() {
		return -1
	} else if job1.GetInstanceName() > job2.GetInstanceName() {
		return 1
	}
	
	return 0
}

// PriorityJobQueue extends JobQueue with prioritization capabilities
type PriorityJobQueue struct {
	*JobQueue
	prioritizer JobPrioritizer
	jobs        []*MigrationJob // Internal priority-ordered slice
	jobChan     chan *MigrationJob
}

// NewPriorityJobQueue creates a new priority-aware job queue
func NewPriorityJobQueue(queueSize int, prioritizer JobPrioritizer) *PriorityJobQueue {
	if prioritizer == nil {
		prioritizer = NewDefaultJobPrioritizer()
	}
	
	return &PriorityJobQueue{
		JobQueue:    NewJobQueue(queueSize),
		prioritizer: prioritizer,
		jobs:        make([]*MigrationJob, 0),
		jobChan:     make(chan *MigrationJob, queueSize),
	}
}

// Enqueue adds a job to the priority queue, maintaining priority order
func (q *PriorityJobQueue) Enqueue(job *MigrationJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.closed {
		return q.JobQueue.Enqueue(job) // Fall back to base implementation
	}
	
	// Insert job in priority order
	insertIndex := q.findInsertPosition(job)
	
	// Insert at the correct position
	q.jobs = append(q.jobs, nil)
	copy(q.jobs[insertIndex+1:], q.jobs[insertIndex:])
	q.jobs[insertIndex] = job
	
	// Try to send to channel if there's space
	select {
	case q.jobChan <- job:
		// Successfully sent to channel
	default:
		// Channel full, job will be retrieved from slice during dequeue
	}
	
	return q.JobQueue.Enqueue(job) // Also track in base queue for metrics
}

// findInsertPosition finds the correct position to insert a job to maintain priority order
func (q *PriorityJobQueue) findInsertPosition(newJob *MigrationJob) int {
	// Binary search for insertion point
	left, right := 0, len(q.jobs)
	
	for left < right {
		mid := (left + right) / 2
		comparison := q.prioritizer.Compare(newJob, q.jobs[mid])
		
		if comparison < 0 {
			// newJob has higher priority, insert before mid
			right = mid
		} else {
			// newJob has equal or lower priority, insert after mid
			left = mid + 1
		}
	}
	
	return left
}

// Dequeue removes and returns the highest priority job from the queue
func (q *PriorityJobQueue) Dequeue() (*MigrationJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.closed {
		return q.JobQueue.Dequeue() // Fall back to base implementation
	}
	
	// Try to get from priority-ordered slice first
	if len(q.jobs) > 0 {
		job := q.jobs[0]
		q.jobs = q.jobs[1:]
		return job, nil
	}
	
	// Fall back to channel if slice is empty
	select {
	case job := <-q.jobChan:
		return job, nil
	default:
		return q.JobQueue.Dequeue() // Fall back to base implementation
	}
}

// SortJobs sorts a slice of jobs using the prioritizer
func SortJobs(jobs []*MigrationJob, prioritizer JobPrioritizer) {
	if prioritizer == nil {
		prioritizer = NewDefaultJobPrioritizer()
	}
	
	// Use a simple but stable sort algorithm
	for i := 0; i < len(jobs)-1; i++ {
		for j := i + 1; j < len(jobs); j++ {
			if prioritizer.Compare(jobs[i], jobs[j]) > 0 {
				jobs[i], jobs[j] = jobs[j], jobs[i]
			}
		}
	}
}