package migrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/gcp"
	"github.com/maxkimambo/pd/internal/logger"
)

// CleanupLevel represents different levels of cleanup granularity
type CleanupLevel int

const (
	// Individual task cleanup - triggered after successful disk restoration
	CleanupLevelTask CleanupLevel = iota
	// Session cleanup - triggered at end of migration session
	CleanupLevelSession
	// Emergency cleanup - triggered for expired snapshots across all sessions
	CleanupLevelEmergency
)

// String returns string representation of CleanupLevel
func (cl CleanupLevel) String() string {
	switch cl {
	case CleanupLevelTask:
		return "task"
	case CleanupLevelSession:
		return "session"
	case CleanupLevelEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

// CleanupStrategy defines the cleanup strategy for snapshots
type CleanupStrategy struct {
	// ImmediateCleanup determines if snapshots should be deleted immediately after disk restoration
	ImmediateCleanup bool
	// SessionCleanup determines if all session snapshots should be cleaned up at migration end
	SessionCleanup bool
	// ScheduledCleanup determines if expired snapshots should be cleaned up
	ScheduledCleanup bool
	// CleanupTimeout is the maximum time to wait for cleanup operations
	CleanupTimeout time.Duration
	// RetryAttempts is the number of retry attempts for failed cleanup operations
	RetryAttempts int
	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration
}

// DefaultCleanupStrategy returns a cleanup strategy with sensible defaults
func DefaultCleanupStrategy() *CleanupStrategy {
	return &CleanupStrategy{
		ImmediateCleanup: true,
		SessionCleanup:   true,
		ScheduledCleanup: true,
		CleanupTimeout:   5 * time.Minute,
		RetryAttempts:    3,
		RetryDelay:       10 * time.Second,
	}
}

// CleanupResult represents the result of a cleanup operation
type CleanupResult struct {
	Level            CleanupLevel
	SessionID        string
	TaskID           string
	SnapshotsFound   int
	SnapshotsDeleted int
	SnapshotsFailed  []string
	Errors           []error
	Duration         time.Duration
}

// MultiLevelCleanupManager handles comprehensive snapshot cleanup across multiple levels
type MultiLevelCleanupManager struct {
	config          *Config
	gcpClient       *gcp.Clients
	strategy        *CleanupStrategy
	sessionID       string
	activeSnapshots map[string]bool // tracks snapshots created in current session
	mu              sync.RWMutex
}

// NewMultiLevelCleanupManager creates a new cleanup manager
func NewMultiLevelCleanupManager(config *Config, gcpClient *gcp.Clients, strategy *CleanupStrategy, sessionID string) *MultiLevelCleanupManager {
	if strategy == nil {
		strategy = DefaultCleanupStrategy()
	}

	return &MultiLevelCleanupManager{
		config:          config,
		gcpClient:       gcpClient,
		strategy:        strategy,
		sessionID:       sessionID,
		activeSnapshots: make(map[string]bool),
	}
}

// RegisterSnapshot registers a snapshot as active in the current session
func (m *MultiLevelCleanupManager) RegisterSnapshot(snapshotName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeSnapshots[snapshotName] = true

	logFields := map[string]interface{}{
		"sessionID":    m.sessionID,
		"snapshotName": snapshotName,
		"totalActive":  len(m.activeSnapshots),
	}
	logger.Op.WithFields(logFields).Debug("Registered active snapshot for cleanup tracking")
}

// UnregisterSnapshot removes a snapshot from active tracking (after successful cleanup)
func (m *MultiLevelCleanupManager) UnregisterSnapshot(snapshotName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.activeSnapshots, snapshotName)

	logFields := map[string]interface{}{
		"sessionID":    m.sessionID,
		"snapshotName": snapshotName,
		"totalActive":  len(m.activeSnapshots),
	}
	logger.Op.WithFields(logFields).Debug("Unregistered snapshot from cleanup tracking")
}

// CleanupTaskSnapshot performs immediate cleanup of a single task snapshot
func (m *MultiLevelCleanupManager) CleanupTaskSnapshot(ctx context.Context, taskID, snapshotName string) *CleanupResult {
	if !m.strategy.ImmediateCleanup {
		return &CleanupResult{
			Level:     CleanupLevelTask,
			SessionID: m.sessionID,
			TaskID:    taskID,
		}
	}

	logFields := map[string]interface{}{
		"sessionID":    m.sessionID,
		"taskID":       taskID,
		"snapshotName": snapshotName,
		"level":        "task",
	}
	logger.Op.WithFields(logFields).Info("Starting task-level snapshot cleanup...")

	startTime := time.Now()
	result := &CleanupResult{
		Level:     CleanupLevelTask,
		SessionID: m.sessionID,
		TaskID:    taskID,
	}

	// Attempt cleanup with retries
	var lastErr error
	for attempt := 0; attempt <= m.strategy.RetryAttempts; attempt++ {
		if attempt > 0 {
			logger.Op.WithFields(logFields).Infof("Retrying task snapshot cleanup (attempt %d/%d)", attempt, m.strategy.RetryAttempts)
			time.Sleep(m.strategy.RetryDelay)
		}

		ctxWithTimeout, cancel := context.WithTimeout(ctx, m.strategy.CleanupTimeout)
		err := m.gcpClient.SnapshotClient.DeleteSnapshot(ctxWithTimeout, m.config.ProjectID, snapshotName)
		cancel()

		if err == nil {
			result.SnapshotsFound = 1
			result.SnapshotsDeleted = 1
			m.UnregisterSnapshot(snapshotName)
			logger.Op.WithFields(logFields).Info("Task snapshot cleanup completed successfully")
			break
		}

		lastErr = err
		logger.Op.WithFields(logFields).WithError(err).Warnf("Task snapshot cleanup attempt %d failed", attempt+1)
	}

	if lastErr != nil {
		result.SnapshotsFailed = []string{snapshotName}
		result.Errors = []error{lastErr}
		logger.Op.WithFields(logFields).WithError(lastErr).Error("Task snapshot cleanup failed after all retry attempts")
	}

	result.Duration = time.Since(startTime)

	return result
}

// CleanupSessionSnapshots performs cleanup of all snapshots in the current session
func (m *MultiLevelCleanupManager) CleanupSessionSnapshots(ctx context.Context) *CleanupResult {
	if !m.strategy.SessionCleanup {
		return &CleanupResult{
			Level:     CleanupLevelSession,
			SessionID: m.sessionID,
		}
	}

	logFields := map[string]interface{}{
		"sessionID": m.sessionID,
		"level":     "session",
	}
	logger.Op.WithFields(logFields).Info("Starting session-level snapshot cleanup...")

	startTime := time.Now()
	result := &CleanupResult{
		Level:     CleanupLevelSession,
		SessionID: m.sessionID,
	}

	// Get all snapshots for this session
	ctxWithTimeout, cancel := context.WithTimeout(ctx, m.strategy.CleanupTimeout)
	snapshots, err := m.gcpClient.SnapshotClient.ListSnapshotsBySessionID(ctxWithTimeout, m.config.ProjectID, m.sessionID)
	cancel()

	if err != nil {
		result.Errors = []error{fmt.Errorf("failed to list session snapshots: %w", err)}
		result.Duration = time.Since(startTime)
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to list session snapshots for cleanup")
		return result
	}

	result.SnapshotsFound = len(snapshots)
	logger.Op.WithFields(logFields).Infof("Found %d snapshots for session cleanup", result.SnapshotsFound)

	if result.SnapshotsFound == 0 {
		logger.Op.WithFields(logFields).Info("No snapshots found for session cleanup")
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform parallel deletion with concurrency control
	concurrencyLimit := m.config.Concurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 50 {
		concurrencyLimit = 10
	}

	semaphore := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	deleteErrors := &sync.Map{}
	deletedCount := int64(0)

	for _, snapshot := range snapshots {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			snapshotLogFields := map[string]interface{}{
				"sessionID":    m.sessionID,
				"snapshotName": snapshotName,
			}

			// Attempt deletion with retries
			var lastErr error
			for attempt := 0; attempt <= m.strategy.RetryAttempts; attempt++ {
				if attempt > 0 {
					time.Sleep(m.strategy.RetryDelay)
				}

				ctxWithTimeout, cancel := context.WithTimeout(ctx, m.strategy.CleanupTimeout)
				err := m.gcpClient.SnapshotClient.DeleteSnapshot(ctxWithTimeout, m.config.ProjectID, snapshotName)
				cancel()

				if err == nil {
					logger.Op.WithFields(snapshotLogFields).Info("Session snapshot deleted successfully")
					m.UnregisterSnapshot(snapshotName)
					deletedCount++
					return
				}

				lastErr = err
				logger.Op.WithFields(snapshotLogFields).WithError(err).Warnf("Session snapshot deletion attempt %d failed", attempt+1)
			}

			deleteErrors.Store(snapshotName, lastErr)
			logger.Op.WithFields(snapshotLogFields).WithError(lastErr).Error("Session snapshot deletion failed after all retry attempts")
		}(snapshot.GetName())
	}

	wg.Wait()

	result.SnapshotsDeleted = int(deletedCount)

	// Collect errors
	deleteErrors.Range(func(key, value interface{}) bool {
		snapshotName := key.(string)
		err := value.(error)
		result.SnapshotsFailed = append(result.SnapshotsFailed, snapshotName)
		result.Errors = append(result.Errors, err)
		return true
	})

	result.Duration = time.Since(startTime)

	resultLogFields := map[string]interface{}{
		"sessionID": m.sessionID,
		"found":     result.SnapshotsFound,
		"deleted":   result.SnapshotsDeleted,
		"failed":    len(result.SnapshotsFailed),
		"duration":  result.Duration,
	}

	if len(result.SnapshotsFailed) > 0 {
		logger.Op.WithFields(resultLogFields).Warnf("Session cleanup completed with %d failures", len(result.SnapshotsFailed))
	} else {
		logger.Op.WithFields(resultLogFields).Info("Session cleanup completed successfully")
	}

	return result
}

// CleanupExpiredSnapshots performs emergency cleanup of all expired snapshots
func (m *MultiLevelCleanupManager) CleanupExpiredSnapshots(ctx context.Context) *CleanupResult {
	if !m.strategy.ScheduledCleanup {
		return &CleanupResult{
			Level:     CleanupLevelEmergency,
			SessionID: m.sessionID,
		}
	}

	logFields := map[string]interface{}{
		"level": "emergency",
	}
	logger.Op.WithFields(logFields).Info("Starting emergency cleanup of expired snapshots...")

	startTime := time.Now()
	result := &CleanupResult{
		Level:     CleanupLevelEmergency,
		SessionID: m.sessionID,
	}

	// Get all expired snapshots
	ctxWithTimeout, cancel := context.WithTimeout(ctx, m.strategy.CleanupTimeout)
	expiredSnapshots, err := m.gcpClient.SnapshotClient.ListExpiredSnapshots(ctxWithTimeout, m.config.ProjectID)
	cancel()

	if err != nil {
		result.Errors = []error{fmt.Errorf("failed to list expired snapshots: %w", err)}
		result.Duration = time.Since(startTime)
		logger.Op.WithFields(logFields).WithError(err).Error("Failed to list expired snapshots for emergency cleanup")
		return result
	}

	result.SnapshotsFound = len(expiredSnapshots)
	logger.Op.WithFields(logFields).Infof("Found %d expired snapshots for emergency cleanup", result.SnapshotsFound)

	if result.SnapshotsFound == 0 {
		logger.Op.WithFields(logFields).Info("No expired snapshots found for emergency cleanup")
		result.Duration = time.Since(startTime)
		return result
	}

	// Perform parallel deletion with concurrency control
	concurrencyLimit := m.config.Concurrency
	if concurrencyLimit <= 0 || concurrencyLimit > 50 {
		concurrencyLimit = 10
	}

	semaphore := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	deleteErrors := &sync.Map{}
	deletedCount := int64(0)

	for _, snapshot := range expiredSnapshots {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(snapshotName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			snapshotLogFields := map[string]interface{}{
				"snapshotName": snapshotName,
			}

			// Attempt deletion with retries
			var lastErr error
			for attempt := 0; attempt <= m.strategy.RetryAttempts; attempt++ {
				if attempt > 0 {
					time.Sleep(m.strategy.RetryDelay)
				}

				ctxWithTimeout, cancel := context.WithTimeout(ctx, m.strategy.CleanupTimeout)
				err := m.gcpClient.SnapshotClient.DeleteSnapshot(ctxWithTimeout, m.config.ProjectID, snapshotName)
				cancel()

				if err == nil {
					logger.Op.WithFields(snapshotLogFields).Info("Expired snapshot deleted successfully")
					deletedCount++
					return
				}

				lastErr = err
				logger.Op.WithFields(snapshotLogFields).WithError(err).Warnf("Expired snapshot deletion attempt %d failed", attempt+1)
			}

			deleteErrors.Store(snapshotName, lastErr)
			logger.Op.WithFields(snapshotLogFields).WithError(lastErr).Error("Expired snapshot deletion failed after all retry attempts")
		}(snapshot.GetName())
	}

	wg.Wait()

	result.SnapshotsDeleted = int(deletedCount)

	// Collect errors
	deleteErrors.Range(func(key, value interface{}) bool {
		snapshotName := key.(string)
		err := value.(error)
		result.SnapshotsFailed = append(result.SnapshotsFailed, snapshotName)
		result.Errors = append(result.Errors, err)
		return true
	})

	result.Duration = time.Since(startTime)

	resultLogFields := map[string]interface{}{
		"found":    result.SnapshotsFound,
		"deleted":  result.SnapshotsDeleted,
		"failed":   len(result.SnapshotsFailed),
		"duration": result.Duration,
	}

	if len(result.SnapshotsFailed) > 0 {
		logger.Op.WithFields(resultLogFields).Warnf("Emergency cleanup completed with %d failures", len(result.SnapshotsFailed))
	} else {
		logger.Op.WithFields(resultLogFields).Info("Emergency cleanup completed successfully")
	}

	return result
}

// GetActiveSnapshots returns a copy of currently tracked active snapshots
func (m *MultiLevelCleanupManager) GetActiveSnapshots() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]string, 0, len(m.activeSnapshots))
	for snapshot := range m.activeSnapshots {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

// GetActiveSnapshotCount returns the number of currently tracked active snapshots
func (m *MultiLevelCleanupManager) GetActiveSnapshotCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeSnapshots)
}
