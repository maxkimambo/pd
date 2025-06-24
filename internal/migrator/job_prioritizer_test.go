package migrator

import (
	"testing"
	"time"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/stretchr/testify/assert"
)

func TestNewDefaultJobPrioritizer(t *testing.T) {
	prioritizer := NewDefaultJobPrioritizer()
	
	assert.NotNil(t, prioritizer)
	assert.True(t, prioritizer.considerMachineType)
	assert.True(t, prioritizer.considerDiskCount)
	assert.True(t, prioritizer.considerDiskSize)
	assert.True(t, prioritizer.considerLabels)
	assert.NotEmpty(t, prioritizer.highPriorityLabels)
	assert.NotEmpty(t, prioritizer.mediumPriorityLabels)
}

func TestNewCustomJobPrioritizer(t *testing.T) {
	highLabels := map[string]string{"priority": "urgent"}
	mediumLabels := map[string]string{"priority": "normal"}
	
	prioritizer := NewCustomJobPrioritizer(
		true, false, true, false,
		highLabels, mediumLabels,
	)
	
	assert.NotNil(t, prioritizer)
	assert.True(t, prioritizer.considerMachineType)
	assert.False(t, prioritizer.considerDiskCount)
	assert.True(t, prioritizer.considerDiskSize)
	assert.False(t, prioritizer.considerLabels)
	assert.Equal(t, highLabels, prioritizer.highPriorityLabels)
	assert.Equal(t, mediumLabels, prioritizer.mediumPriorityLabels)
}

func TestDefaultJobPrioritizer_AssignPriority_ByLabels(t *testing.T) {
	// Use a custom prioritizer that only considers labels
	prioritizer := NewCustomJobPrioritizer(
		false, false, false, true, // Only consider labels
		map[string]string{
			"environment": "production",
			"criticality": "high",
			"priority":    "high",
		},
		map[string]string{
			"environment": "staging",
			"criticality": "medium",
			"priority":    "medium",
		},
	)
	
	tests := []struct {
		name     string
		labels   map[string]string
		expected JobPriority
	}{
		{
			name: "High priority production label",
			labels: map[string]string{
				"environment": "production",
				"app":         "web-server",
			},
			expected: HighPriority,
		},
		{
			name: "High priority criticality label",
			labels: map[string]string{
				"criticality": "high",
				"team":        "backend",
			},
			expected: HighPriority,
		},
		{
			name: "Medium priority staging label",
			labels: map[string]string{
				"environment": "staging",
			},
			expected: MediumPriority,
		},
		{
			name: "Low priority no matching labels",
			labels: map[string]string{
				"team": "dev",
				"app":  "test",
			},
			expected: LowPriority,
		},
		{
			name:     "Low priority no labels",
			labels:   nil,
			expected: LowPriority,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &computepb.Instance{
				Labels: tt.labels,
			}
			disks := []*AttachedDiskInfo{}
			
			priority := prioritizer.AssignPriority(instance, disks)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestDefaultJobPrioritizer_AssignPriority_ByMachineType(t *testing.T) {
	prioritizer := NewCustomJobPrioritizer(
		true, false, false, false, // Only consider machine type
		nil, nil,
	)
	
	tests := []struct {
		name        string
		machineType string
		expected    JobPriority
	}{
		{
			name:        "High priority micro instance",
			machineType: "projects/test/zones/us-west1-a/machineTypes/f1-micro",
			expected:    HighPriority,
		},
		{
			name:        "High priority small instance",
			machineType: "g1-small",
			expected:    HighPriority,
		},
		{
			name:        "Medium priority standard instance",
			machineType: "n1-standard-1",
			expected:    MediumPriority,
		},
		{
			name:        "Low priority large instance",
			machineType: "n1-highmem-32",
			expected:    LowPriority,
		},
		{
			name:        "Medium priority unknown type",
			machineType: "custom-type",
			expected:    MediumPriority,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &computepb.Instance{
				MachineType: &tt.machineType,
			}
			disks := []*AttachedDiskInfo{}
			
			priority := prioritizer.AssignPriority(instance, disks)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestDefaultJobPrioritizer_AssignPriority_ByDiskCount(t *testing.T) {
	prioritizer := NewCustomJobPrioritizer(
		false, true, false, false, // Only consider disk count
		nil, nil,
	)
	
	tests := []struct {
		name      string
		diskCount int
		expected  JobPriority
	}{
		{
			name:      "High priority many disks",
			diskCount: 5,
			expected:  HighPriority,
		},
		{
			name:      "High priority more disks",
			diskCount: 10,
			expected:  HighPriority,
		},
		{
			name:      "Medium priority few disks",
			diskCount: 2,
			expected:  MediumPriority,
		},
		{
			name:      "Medium priority three disks",
			diskCount: 3,
			expected:  MediumPriority,
		},
		{
			name:      "Low priority single disk",
			diskCount: 1,
			expected:  LowPriority,
		},
		{
			name:      "Low priority no disks",
			diskCount: 0,
			expected:  LowPriority,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &computepb.Instance{}
			disks := make([]*AttachedDiskInfo, tt.diskCount)
			
			priority := prioritizer.AssignPriority(instance, disks)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestDefaultJobPrioritizer_AssignPriority_ByDiskSize(t *testing.T) {
	prioritizer := NewCustomJobPrioritizer(
		false, false, true, false, // Only consider disk size
		nil, nil,
	)
	
	tests := []struct {
		name     string
		diskSizes []int64 // in GB
		expected JobPriority
	}{
		{
			name:      "Low priority very large disk",
			diskSizes: []int64{15000}, // 15TB
			expected:  LowPriority,
		},
		{
			name:      "Low priority multiple large disks",
			diskSizes: []int64{5000, 6000}, // 11TB total
			expected:  LowPriority,
		},
		{
			name:      "Medium priority medium disk",
			diskSizes: []int64{2000}, // 2TB
			expected:  MediumPriority,
		},
		{
			name:      "Medium priority multiple medium disks",
			diskSizes: []int64{500, 700}, // 1.2TB total
			expected:  MediumPriority,
		},
		{
			name:      "High priority small disk",
			diskSizes: []int64{100}, // 100GB
			expected:  HighPriority,
		},
		{
			name:      "High priority multiple small disks",
			diskSizes: []int64{50, 200, 300}, // 550GB total
			expected:  HighPriority,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &computepb.Instance{}
			disks := make([]*AttachedDiskInfo, len(tt.diskSizes))
			
			for i, size := range tt.diskSizes {
				disks[i] = &AttachedDiskInfo{
					DiskDetails: &computepb.Disk{
						SizeGb: &size,
					},
				}
			}
			
			priority := prioritizer.AssignPriority(instance, disks)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestDefaultJobPrioritizer_Compare(t *testing.T) {
	prioritizer := NewDefaultJobPrioritizer()
	
	now := time.Now()
	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)
	
	tests := []struct {
		name     string
		job1     *MigrationJob
		job2     *MigrationJob
		expected int
	}{
		{
			name: "Job1 higher priority than Job2",
			job1: &MigrationJob{
				Priority:  HighPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
			},
			expected: -1,
		},
		{
			name: "Job2 higher priority than Job1",
			job1: &MigrationJob{
				Priority:  LowPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  HighPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
			},
			expected: 1,
		},
		{
			name: "Same priority, Job1 created earlier",
			job1: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: earlier,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: later,
				Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
			},
			expected: -1,
		},
		{
			name: "Same priority, Job2 created earlier",
			job1: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: later,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: earlier,
				Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
			},
			expected: 1,
		},
		{
			name: "Same priority and time, compare by name",
			job1: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
			},
			expected: -1,
		},
		{
			name: "Identical jobs",
			job1: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			job2: &MigrationJob{
				Priority:  MediumPriority,
				CreatedAt: now,
				Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
			},
			expected: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prioritizer.Compare(tt.job1, tt.job2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractMachineTypeName(t *testing.T) {
	tests := []struct {
		name        string
		machineType string
		expected    string
	}{
		{
			name:        "Full GCP URL",
			machineType: "projects/test-project/zones/us-west1-a/machineTypes/n1-standard-1",
			expected:    "n1-standard-1",
		},
		{
			name:        "Simple name",
			machineType: "f1-micro",
			expected:    "f1-micro",
		},
		{
			name:        "Empty string",
			machineType: "",
			expected:    "",
		},
		{
			name:        "URL with trailing slash",
			machineType: "projects/test/zones/us-west1-a/machineTypes/g1-small/",
			expected:    "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMachineTypeName(tt.machineType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSortJobs(t *testing.T) {
	prioritizer := NewDefaultJobPrioritizer()
	
	now := time.Now()
	
	jobs := []*MigrationJob{
		{
			ID:        "job-3",
			Priority:  LowPriority,
			CreatedAt: now,
			Instance:  &computepb.Instance{Name: stringPtr("instance-c")},
		},
		{
			ID:        "job-1",
			Priority:  HighPriority,
			CreatedAt: now.Add(-time.Hour),
			Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
		},
		{
			ID:        "job-2",
			Priority:  MediumPriority,
			CreatedAt: now,
			Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
		},
		{
			ID:        "job-4",
			Priority:  HighPriority,
			CreatedAt: now,
			Instance:  &computepb.Instance{Name: stringPtr("instance-d")},
		},
	}
	
	SortJobs(jobs, prioritizer)
	
	// Expected order: job-1 (High, earlier), job-4 (High, later), job-2 (Medium), job-3 (Low)
	assert.Equal(t, "job-1", jobs[0].ID)
	assert.Equal(t, "job-4", jobs[1].ID)
	assert.Equal(t, "job-2", jobs[2].ID)
	assert.Equal(t, "job-3", jobs[3].ID)
}

func TestSortJobs_WithNilPrioritizer(t *testing.T) {
	now := time.Now()
	
	jobs := []*MigrationJob{
		{
			ID:        "job-2",
			Priority:  LowPriority,
			CreatedAt: now,
			Instance:  &computepb.Instance{Name: stringPtr("instance-b")},
		},
		{
			ID:        "job-1",
			Priority:  HighPriority,
			CreatedAt: now,
			Instance:  &computepb.Instance{Name: stringPtr("instance-a")},
		},
	}
	
	// Should not panic with nil prioritizer
	SortJobs(jobs, nil)
	
	// Should be sorted by priority (High first)
	assert.Equal(t, "job-1", jobs[0].ID)
	assert.Equal(t, "job-2", jobs[1].ID)
}

func TestNewPriorityJobQueue(t *testing.T) {
	prioritizer := NewDefaultJobPrioritizer()
	queue := NewPriorityJobQueue(10, prioritizer)
	
	assert.NotNil(t, queue)
	assert.NotNil(t, queue.JobQueue)
	assert.Equal(t, prioritizer, queue.prioritizer)
	assert.NotNil(t, queue.jobs)
	assert.NotNil(t, queue.jobChan)
}

func TestNewPriorityJobQueue_WithNilPrioritizer(t *testing.T) {
	queue := NewPriorityJobQueue(10, nil)
	
	assert.NotNil(t, queue)
	assert.NotNil(t, queue.prioritizer)
	assert.IsType(t, &DefaultJobPrioritizer{}, queue.prioritizer)
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}