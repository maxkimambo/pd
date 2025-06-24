package dag

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/progress"
)

// ExecutorConfig contains configuration for the DAG executor
type ExecutorConfig struct {
	// MaxParallelTasks is the maximum number of tasks to run in parallel
	MaxParallelTasks int

	// TaskTimeout is the timeout for both individual tasks and dependencies
	TaskTimeout time.Duration

	// PollInterval is how often to check dependency completion
	PollInterval time.Duration
}

// DefaultExecutorConfig returns a default configuration
func DefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{
		MaxParallelTasks: 10,
		TaskTimeout:      15 * time.Minute,
		PollInterval:     100 * time.Millisecond,
	}
}

// ExecutionResult contains the results of DAG execution
type ExecutionResult struct {
	// Success indicates if the entire DAG executed successfully
	Success bool

	// NodeResults maps node IDs to their execution results
	NodeResults map[string]*NodeResult

	// ExecutionTime is the total time taken for execution
	ExecutionTime time.Duration

	// Error is the first error encountered during execution
	Error error
}

// NodeResult contains the result of a single node execution
type NodeResult struct {
	// NodeID is the ID of the node
	NodeID string

	// Success indicates if the node executed successfully
	Success bool

	// Error is any error that occurred during execution
	Error error

	// StartTime is when the node started executing
	StartTime *time.Time

	// EndTime is when the node finished executing
	EndTime *time.Time

	// Duration is how long the node took to execute
	Duration time.Duration
}

// Executor handles the execution of a DAG
type Executor struct {
	dag       *DAG
	config    *ExecutorConfig
	workers   chan struct{}
	results   map[string]*NodeResult
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startTime time.Time
	finished  chan struct{}
}

// NewExecutor creates a new DAG executor
func NewExecutor(dag *DAG, config *ExecutorConfig) *Executor {
	if config == nil {
		config = DefaultExecutorConfig()
	}

	return &Executor{
		dag:      dag,
		config:   config,
		workers:  make(chan struct{}, config.MaxParallelTasks),
		results:  make(map[string]*NodeResult),
		finished: make(chan struct{}),
	}
}

// Execute runs the DAG to completion
func (e *Executor) Execute(ctx context.Context) (*ExecutionResult, error) {
	e.startTime = time.Now()

	// Set context with proper synchronization
	e.mutex.Lock()
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.mutex.Unlock()

	defer e.cancel()

	// Validate DAG before execution
	if err := e.dag.Validate(); err != nil {
		e.initializeResults()
		return e.buildResult(), fmt.Errorf("invalid DAG: %w", err)
	}

	// Initialize results for all nodes
	e.initializeResults()

	// Start execution with root nodes
	rootNodes, err := e.dag.GetRootNodes()
	if err != nil {
		return e.buildResult(), fmt.Errorf("failed to get root nodes: %w", err)
	}

	if len(rootNodes) == 0 {
		return e.buildResult(), fmt.Errorf("no root nodes found in DAG")
	}

	// Log execution start
	totalNodes := len(e.dag.GetAllNodes())
	if logger.User != nil {
		logger.User.Infof("Starting execution of %d tasks (max %d parallel)", totalNodes, e.config.MaxParallelTasks)
	}

	// Start progress logging goroutine
	go e.logProgress()

	// Execute root nodes
	for _, nodeID := range rootNodes {
		e.scheduleNode(nodeID)
	}

	// Wait for all nodes to complete
	e.wg.Wait()

	// Signal that execution is finished
	close(e.finished)

	// Log final progress
	e.logFinalProgress()

	return e.buildResult(), nil
}

// Cancel cancels the DAG execution
func (e *Executor) Cancel() {
	e.mutex.RLock()
	cancel := e.cancel
	e.mutex.RUnlock()

	if cancel != nil {
		cancel()
	}
}

// GetProgress returns the current execution progress
func (e *Executor) GetProgress() (completed, total int) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	total = len(e.results)
	for _, result := range e.results {
		if result.Success || result.Error != nil {
			completed++
		}
	}

	return completed, total
}

// initializeResults creates result entries for all nodes
func (e *Executor) initializeResults() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	allNodes := e.dag.GetAllNodes()
	for _, nodeID := range allNodes {
		e.results[nodeID] = &NodeResult{
			NodeID: nodeID,
		}
	}
}

// scheduleNode schedules a node for execution
func (e *Executor) scheduleNode(nodeID string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Check if already scheduled/started to prevent double scheduling
	result := e.results[nodeID]
	if result.StartTime != nil {
		return // Already scheduled or started
	}

	// Mark as started immediately to prevent race conditions
	now := time.Now()
	result.StartTime = &now

	e.wg.Add(1)
	go e.executeNode(nodeID)
}

// executeNode executes a single node
func (e *Executor) executeNode(nodeID string) {
	defer e.wg.Done()

	// Acquire worker slot
	select {
	case e.workers <- struct{}{}:
		defer func() { <-e.workers }()
	case <-e.ctx.Done():
		e.setNodeError(nodeID, e.ctx.Err())
		return
	}

	// Get the node
	node, err := e.dag.GetNode(nodeID)
	if err != nil {
		e.setNodeError(nodeID, err)
		return
	}

	// Wait for dependencies to complete
	if !e.waitForDependencies(nodeID) {
		e.setNodeError(nodeID, fmt.Errorf("dependencies failed or timed out"))
		return
	}

	// Check if execution was cancelled
	select {
	case <-e.ctx.Done():
		e.setNodeError(nodeID, e.ctx.Err())
		return
	default:
	}

	// Execute the node with timeout
	nodeCtx, cancel := context.WithTimeout(e.ctx, e.config.TaskTimeout)
	defer cancel()

	e.setNodeStarted(nodeID)

	// Log task start
	if logger.User != nil {
		task := node.GetTask()
		logger.User.Infof("Starting task: %s (%s)", nodeID, GetTaskType(task))
	}

	err = node.Execute(nodeCtx)

	// Log task completion
	if logger.User != nil {
		if err != nil {
			logger.User.Errorf("Task failed: %s - %v", nodeID, err)
		} else {
			task := node.GetTask()
			logger.User.Successf("Task completed: %s (%s)", nodeID, GetTaskType(task))
		}
	}

	e.setNodeCompleted(nodeID, err)

	// If execution was successful, schedule dependent nodes
	if err == nil {
		e.scheduleDependents(nodeID)
	} else {
		// If execution failed, propagate failure to dependents
		e.cancelDependents(nodeID)
	}
}

// waitForDependencies waits for all dependencies of a node to complete
func (e *Executor) waitForDependencies(nodeID string) bool {
	deps, err := e.dag.GetDependencies(nodeID)
	if err != nil {
		return false
	}

	// No dependencies, can execute immediately
	if len(deps) == 0 {
		return true
	}

	// Log dependency waiting
	if logger.Op != nil && len(deps) > 0 {
		logger.Op.WithFields(map[string]interface{}{
			"task":         nodeID,
			"dependencies": deps,
		}).Debug("Waiting for dependencies to complete")
	}

	timeout := time.After(e.config.TaskTimeout)
	ticker := time.NewTicker(e.config.PollInterval)
	defer ticker.Stop()

	dependencyLogTimer := time.NewTicker(30 * time.Second)
	defer dependencyLogTimer.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return false
		case <-timeout:
			if logger.User != nil {
				logger.User.Warnf("Task %s timed out waiting for dependencies: %v", nodeID, deps)
			}
			return false
		case <-dependencyLogTimer.C:
			// Log still waiting for dependencies every 30 seconds
			if logger.User != nil {
				pending := e.getPendingDependencies(deps)
				if len(pending) > 0 {
					logger.User.Infof("Task %s still waiting for dependencies: %v", nodeID, pending)
				}
			}
		case <-ticker.C:
			if e.areDependenciesCompleted(deps) {
				if logger.Op != nil {
					logger.Op.WithFields(map[string]interface{}{
						"task": nodeID,
					}).Debug("All dependencies completed, proceeding with execution")
				}
				return true
			}
		}
	}
}

// areDependenciesCompleted checks if all dependencies have completed successfully
func (e *Executor) areDependenciesCompleted(deps []string) bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, depID := range deps {
		result, exists := e.results[depID]
		if !exists || !result.Success || result.Error != nil {
			return false
		}
	}

	return true
}

// getPendingDependencies returns the list of dependencies that are still pending
func (e *Executor) getPendingDependencies(deps []string) []string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	var pending []string
	for _, depID := range deps {
		result, exists := e.results[depID]
		if !exists || (!result.Success && result.Error == nil) {
			pending = append(pending, depID)
		}
	}

	return pending
}

// scheduleDependents schedules all dependent nodes that are ready to execute
func (e *Executor) scheduleDependents(nodeID string) {
	dependents, err := e.dag.GetDependents(nodeID)
	if err != nil {
		return
	}

	for _, depID := range dependents {
		if e.isNodeReady(depID) {
			e.scheduleNode(depID)
		}
	}
}

// isNodeReady checks if a node is ready to execute (all dependencies completed)
func (e *Executor) isNodeReady(nodeID string) bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Check if node has already been started
	result := e.results[nodeID]
	if result.StartTime != nil {
		return false // Already started or completed
	}

	// Check dependencies
	deps, err := e.dag.GetDependencies(nodeID)
	if err != nil {
		return false
	}

	return e.areDependenciesCompleted(deps)
}

// cancelDependents marks all dependent nodes as cancelled
func (e *Executor) cancelDependents(nodeID string) {
	dependents, err := e.dag.GetDependents(nodeID)
	if err != nil {
		return
	}

	for _, depID := range dependents {
		e.setNodeError(depID, fmt.Errorf("cancelled due to dependency %s failure", nodeID))
		e.cancelDependents(depID) // Recursively cancel
	}
}

// setNodeStarted marks a node as started
func (e *Executor) setNodeStarted(nodeID string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if result, exists := e.results[nodeID]; exists {
		// Only set StartTime if not already set (to preserve scheduling time)
		if result.StartTime == nil {
			now := time.Now()
			result.StartTime = &now
		}
	}
}

// setNodeCompleted marks a node as completed with the given error
func (e *Executor) setNodeCompleted(nodeID string, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if result, exists := e.results[nodeID]; exists {
		now := time.Now()
		result.EndTime = &now
		result.Error = err
		result.Success = (err == nil)

		if result.StartTime != nil {
			result.Duration = now.Sub(*result.StartTime)
		}
	}
}

// setNodeError marks a node as failed with the given error
func (e *Executor) setNodeError(nodeID string, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if result, exists := e.results[nodeID]; exists {
		now := time.Now()
		if result.StartTime == nil {
			result.StartTime = &now
		}
		result.EndTime = &now
		result.Error = err
		result.Success = false

		if result.StartTime != nil {
			result.Duration = now.Sub(*result.StartTime)
		}
	}
}

// buildResult constructs the final execution result
func (e *Executor) buildResult() *ExecutionResult {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	result := &ExecutionResult{
		NodeResults:   make(map[string]*NodeResult),
		ExecutionTime: time.Since(e.startTime),
		Success:       len(e.results) > 0, // Success only if there are nodes and none failed
	}

	// Copy node results and check for failures
	for nodeID, nodeResult := range e.results {
		// Create a copy of the node result
		resultCopy := *nodeResult
		result.NodeResults[nodeID] = &resultCopy

		// Check if this node failed
		if nodeResult.Error != nil {
			result.Success = false
			if result.Error == nil {
				result.Error = fmt.Errorf("node %s failed: %w", nodeID, nodeResult.Error)
			}
		}
	}

	return result
}

// IsRunning returns true if the executor is currently running
func (e *Executor) IsRunning() bool {
	e.mutex.RLock()
	ctx := e.ctx
	e.mutex.RUnlock()

	if ctx == nil {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

// logProgress provides periodic progress updates during execution
func (e *Executor) logProgress() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.finished:
			return
		case <-ticker.C:
			e.printProgress()
		}
	}
}

// printProgress logs current execution progress with enhanced details
func (e *Executor) printProgress() {
	completed, total := e.GetProgress()
	if total > 0 && logger.User != nil {
		// Create enhanced progress info
		progressInfo := e.buildProgressInfo(completed, total)
		
		// Use enhanced reporter
		reporter := progress.NewReporter()
		progressReport := reporter.Report(progressInfo)
		
		logger.User.Info(progressReport)
	}
}

// buildProgressInfo creates detailed progress information
func (e *Executor) buildProgressInfo(completed, total int) progress.ProgressInfo {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	
	elapsed := time.Since(e.startTime)
	
	// Calculate task breakdown by type
	taskBreakdown := make(map[progress.TaskType]progress.TaskStats)
	instanceStats := make(map[string]progress.InstanceProgress)
	runningCount := 0
	failedCount := 0
	pendingCount := 0
	currentOperation := ""
	
	// Get all node IDs from the DAG
	nodeIDs := e.dag.GetAllNodes()
	
	// Analyze running tasks and task types
	for _, nodeID := range nodeIDs {
		node, err := e.dag.GetNode(nodeID)
		if err != nil {
			continue
		}
		
		task := node.GetTask()
		if task != nil {
			taskType := e.getTaskType(task.GetID())
			
			if _, exists := taskBreakdown[taskType]; !exists {
				taskBreakdown[taskType] = progress.TaskStats{}
			}
			stats := taskBreakdown[taskType]
			stats.Total++
			
			if result, hasResult := e.results[nodeID]; hasResult {
				if result.Success {
					stats.Completed++
				} else {
					stats.Failed++
					failedCount++
				}
			} else {
				// Task is pending (not started yet)
				stats.Pending++
				pendingCount++
			}
			
			taskBreakdown[taskType] = stats
			
			// Track instance progress
			instanceName := e.extractInstanceName(task.GetID())
			if instanceName != "" {
				if _, exists := instanceStats[instanceName]; !exists {
					instanceStats[instanceName] = progress.InstanceProgress{
						InstanceName: instanceName,
						Status:       "pending",
						CurrentPhase: progress.PhaseDiscovery,
					}
				}
				
				instanceProg := instanceStats[instanceName]
				taskPhase := e.getTaskPhase(task.GetID())
				
				if result, hasResult := e.results[nodeID]; hasResult {
					if result.Success {
						instanceProg.DisksProcessed++
						instanceProg.Status = "completed"
						// Update to the latest completed phase for this instance
						instanceProg.CurrentPhase = taskPhase
					} else {
						instanceProg.Status = "failed"
						instanceProg.CurrentPhase = taskPhase
					}
				} else {
					// Task is pending - only update phase if this is earlier than current
					if e.isEarlierPhase(taskPhase, instanceProg.CurrentPhase) {
						instanceProg.CurrentPhase = taskPhase
					}
					// Keep existing status if already processing/failed
					if instanceProg.Status == "pending" {
						instanceProg.Status = "pending"
					}
				}
				
				instanceStats[instanceName] = instanceProg
			}
		}
	}
	
	// Calculate ETA
	eta := progress.CalculateETA(completed, total, elapsed)
	
	return progress.ProgressInfo{
		CurrentPhase:      e.getCurrentPhase(currentOperation),
		TotalTasks:        total,
		CompletedTasks:    completed,
		FailedTasks:       failedCount,
		RunningTasks:      runningCount,
		ElapsedTime:       elapsed,
		EstimatedTimeLeft: eta,
		TaskBreakdown:     taskBreakdown,
		CurrentOperation:  currentOperation,
		InstanceStats:     instanceStats,
	}
}

// getTaskType determines the task type from task ID
func (e *Executor) getTaskType(taskID string) progress.TaskType {
	if strings.Contains(taskID, "snapshot") {
		return progress.TaskTypeSnapshot
	} else if strings.Contains(taskID, "shutdown") || strings.Contains(taskID, "startup") {
		return progress.TaskTypeInstanceState
	} else if strings.Contains(taskID, "attach") || strings.Contains(taskID, "detach") {
		return progress.TaskTypeDiskAttachment
	} else if strings.Contains(taskID, "migrate") {
		return progress.TaskTypeDiskMigration
	} else if strings.Contains(taskID, "cleanup") {
		return progress.TaskTypeCleanup
	}
	return progress.TaskType("Unknown")
}

// getCurrentPhase determines the current migration phase
func (e *Executor) getCurrentPhase(currentOperation string) progress.Phase {
	if strings.Contains(currentOperation, "snapshot") {
		return progress.PhaseSnapshot
	} else if strings.Contains(currentOperation, "shutdown") {
		return progress.PhaseShutdown
	} else if strings.Contains(currentOperation, "detach") {
		return progress.PhaseDetach
	} else if strings.Contains(currentOperation, "migrate") {
		return progress.PhaseMigration
	} else if strings.Contains(currentOperation, "attach") {
		return progress.PhaseAttach
	} else if strings.Contains(currentOperation, "startup") {
		return progress.PhaseStartup
	} else if strings.Contains(currentOperation, "cleanup") {
		return progress.PhaseCleanup
	}
	return progress.Phase("Processing")
}

// getTaskPhase determines the phase for a task
func (e *Executor) getTaskPhase(taskID string) progress.Phase {
	return e.getCurrentPhase(taskID)
}

// isEarlierPhase determines if phase1 comes before phase2 in the migration sequence
func (e *Executor) isEarlierPhase(phase1, phase2 progress.Phase) bool {
	phaseOrder := map[progress.Phase]int{
		progress.PhaseDiscovery:  1,
		progress.PhaseValidation: 2,
		progress.PhaseSnapshot:   3,
		progress.PhaseShutdown:   4,
		progress.PhaseDetach:     5,
		progress.PhaseMigration:  6,
		progress.PhaseAttach:     7,
		progress.PhaseStartup:    8,
		progress.PhaseCleanup:    9,
		progress.PhaseCompletion: 10,
	}
	
	order1, exists1 := phaseOrder[phase1]
	order2, exists2 := phaseOrder[phase2]
	
	if !exists1 || !exists2 {
		return false
	}
	
	return order1 < order2
}

// extractInstanceName extracts instance name from task ID
func (e *Executor) extractInstanceName(taskID string) string {
	// Task IDs typically follow pattern: operation_instance_disk
	parts := strings.Split(taskID, "_")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}


// logFinalProgress logs the final execution summary
func (e *Executor) logFinalProgress() {
	completed, total := e.GetProgress()
	elapsed := time.Since(e.startTime)

	if logger.User != nil {
		// Count failed tasks
		failed := 0
		e.mutex.RLock()
		for _, result := range e.results {
			if result.Error != nil {
				failed++
			}
		}
		e.mutex.RUnlock()

		if failed == 0 {
			logger.User.Successf("Execution completed: %d/%d tasks successful in %v",
				completed, total, elapsed.Round(time.Second))
		} else {
			logger.User.Errorf("Execution completed: %d successful, %d failed in %v",
				completed-failed, failed, elapsed.Round(time.Second))
		}
	}
}
