package migrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
)

// ResourceType represents the type of resource being locked
type ResourceType string

const (
	ResourceTypeInstance ResourceType = "instance"
	ResourceTypeDisk     ResourceType = "disk"
	ResourceTypeSnapshot ResourceType = "snapshot"
)

// ResourceID represents a unique resource identifier
type ResourceID struct {
	Type     ResourceType
	Project  string
	Zone     string
	Name     string
}

// String returns a string representation of the resource ID
func (r ResourceID) String() string {
	return fmt.Sprintf("%s:%s/%s/%s", r.Type, r.Project, r.Zone, r.Name)
}

// ResourceLock represents a lock on a specific resource
type ResourceLock struct {
	ResourceID    ResourceID
	LockedAt      time.Time
	LockedBy      string
	JobID         string
}

// ResourceLocker provides distributed resource locking to prevent conflicts
type ResourceLocker struct {
	locks map[string]*ResourceLock
	mutex sync.RWMutex
}

// NewResourceLocker creates a new resource locker
func NewResourceLocker() *ResourceLocker {
	return &ResourceLocker{
		locks: make(map[string]*ResourceLock),
	}
}

// LockResources attempts to acquire locks on multiple resources atomically
// If any resource is already locked, all locks are released and an error is returned
func (rl *ResourceLocker) LockResources(ctx context.Context, resources []ResourceID, jobID string) error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Check if any resources are already locked
	for _, resource := range resources {
		key := resource.String()
		if lock, exists := rl.locks[key]; exists {
			logger.Op.WithFields(map[string]interface{}{
				"resourceID":     key,
				"currentJobID":   jobID,
				"conflictJobID":  lock.JobID,
				"lockedBy":       lock.LockedBy,
				"lockedAt":       lock.LockedAt,
			}).Warn("Resource conflict detected - resource already locked")
			
			return fmt.Errorf("resource %s is already locked by job %s", key, lock.JobID)
		}
	}

	// Check for context cancellation before acquiring locks
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Acquire all locks atomically
	lockedBy := fmt.Sprintf("job-%s", jobID)
	now := time.Now()
	
	for _, resource := range resources {
		key := resource.String()
		rl.locks[key] = &ResourceLock{
			ResourceID: resource,
			LockedAt:   now,
			LockedBy:   lockedBy,
			JobID:      jobID,
		}
		
		logger.Op.WithFields(map[string]interface{}{
			"resourceID": key,
			"jobID":      jobID,
			"lockedBy":   lockedBy,
		}).Debug("Resource locked")
	}

	logger.Op.WithFields(map[string]interface{}{
		"jobID":           jobID,
		"resourceCount":   len(resources),
		"lockedBy":        lockedBy,
	}).Info("Successfully locked all resources")

	return nil
}

// UnlockResources releases locks on multiple resources
func (rl *ResourceLocker) UnlockResources(resources []ResourceID, jobID string) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	for _, resource := range resources {
		key := resource.String()
		if lock, exists := rl.locks[key]; exists {
			if lock.JobID == jobID {
				delete(rl.locks, key)
				logger.Op.WithFields(map[string]interface{}{
					"resourceID": key,
					"jobID":      jobID,
				}).Debug("Resource unlocked")
			} else {
				logger.Op.WithFields(map[string]interface{}{
					"resourceID":    key,
					"jobID":         jobID,
					"actualJobID":   lock.JobID,
				}).Warn("Attempted to unlock resource locked by different job")
			}
		}
	}

	logger.Op.WithFields(map[string]interface{}{
		"jobID":         jobID,
		"resourceCount": len(resources),
	}).Info("Released resource locks")
}

// IsLocked checks if a resource is currently locked
func (rl *ResourceLocker) IsLocked(resource ResourceID) (bool, *ResourceLock) {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	key := resource.String()
	if lock, exists := rl.locks[key]; exists {
		return true, lock
	}
	return false, nil
}

// GetLockInfo returns information about all current locks
func (rl *ResourceLocker) GetLockInfo() map[string]*ResourceLock {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	result := make(map[string]*ResourceLock, len(rl.locks))
	for key, lock := range rl.locks {
		// Create a copy to avoid race conditions
		lockCopy := *lock
		result[key] = &lockCopy
	}
	return result
}

// CleanupStaleLocksOlderThan removes locks older than the specified duration
// This helps clean up locks from failed or cancelled jobs
func (rl *ResourceLocker) CleanupStaleLocksOlderThan(duration time.Duration) int {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	cutoff := time.Now().Add(-duration)
	var removedCount int

	for key, lock := range rl.locks {
		if lock.LockedAt.Before(cutoff) {
			delete(rl.locks, key)
			removedCount++
			logger.Op.WithFields(map[string]interface{}{
				"resourceID": key,
				"jobID":      lock.JobID,
				"lockedAt":   lock.LockedAt,
				"duration":   time.Since(lock.LockedAt),
			}).Info("Cleaned up stale resource lock")
		}
	}

	if removedCount > 0 {
		logger.Op.WithFields(map[string]interface{}{
			"removedCount": removedCount,
			"cutoffAge":    duration,
		}).Info("Cleaned up stale resource locks")
	}

	return removedCount
}

// GetResourceIDsForInstanceMigration extracts all resources that need to be locked for an instance migration
func GetResourceIDsForInstanceMigration(migration *InstanceMigration, config *Config) []ResourceID {
	var resources []ResourceID

	if migration.Instance == nil {
		return resources
	}

	// Add the instance itself
	resources = append(resources, ResourceID{
		Type:    ResourceTypeInstance,
		Project: config.ProjectID,
		Zone:    extractZoneFromInstance(migration.Instance),
		Name:    migration.Instance.GetName(),
	})

	// Add all attached disks
	for _, diskInfo := range migration.Disks {
		if diskInfo.DiskDetails != nil {
			resources = append(resources, ResourceID{
				Type:    ResourceTypeDisk,
				Project: config.ProjectID,
				Zone:    extractZoneFromInstance(migration.Instance),
				Name:    diskInfo.DiskDetails.GetName(),
			})
		}
	}

	return resources
}

// extractZoneFromInstance extracts zone name from instance zone URL
func extractZoneFromInstance(instance *computepb.Instance) string {
	if instance == nil {
		return ""
	}
	zone := instance.GetZone()
	if zone == "" {
		return ""
	}
	// Extract zone name from URL like "projects/PROJECT/zones/us-west1-a"
	parts := strings.Split(zone, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return zone
}