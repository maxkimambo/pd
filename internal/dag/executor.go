package dag

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
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
	dag           *DAG
	config        *ExecutorConfig
	workers       chan struct{}
	results       map[string]*NodeResult
	mutex         sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	startTime     time.Time
	visualization *DAGVisualization
	visualizeFile string
	visualizeTicker *time.Ticker
	finished      chan struct{}
}

// NewExecutor creates a new DAG executor
func NewExecutor(dag *DAG, config *ExecutorConfig) *Executor {
	if config == nil {
		config = DefaultExecutorConfig()
	}
	
	return &Executor{
		dag:           dag,
		config:        config,
		workers:       make(chan struct{}, config.MaxParallelTasks),
		results:       make(map[string]*NodeResult),
		visualization: NewDAGVisualization(dag),
		finished:      make(chan struct{}),
	}
}

// Execute runs the DAG to completion
func (e *Executor) Execute(ctx context.Context) (*ExecutionResult, error) {
	e.startTime = time.Now()
	e.ctx, e.cancel = context.WithCancel(ctx)
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
	
	// Execute root nodes
	for _, nodeID := range rootNodes {
		e.scheduleNode(nodeID)
	}
	
	// Wait for all nodes to complete
	e.wg.Wait()
	
	// Signal that execution is finished
	close(e.finished)
	
	// Stop visualization updates if running
	if e.visualizeTicker != nil {
		e.visualizeTicker.Stop()
	}
	
	// Final visualization update
	e.updateVisualization()
	
	return e.buildResult(), nil
}

// Cancel cancels the DAG execution
func (e *Executor) Cancel() {
	if e.cancel != nil {
		e.cancel()
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
	
	err = node.Execute(nodeCtx)
	
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
	
	timeout := time.After(e.config.TaskTimeout)
	ticker := time.NewTicker(e.config.PollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-e.ctx.Done():
			return false
		case <-timeout:
			return false
		case <-ticker.C:
			if e.areDependenciesCompleted(deps) {
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
		now := time.Now()
		result.StartTime = &now
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
	if e.ctx == nil {
		return false
	}
	
	select {
	case <-e.ctx.Done():
		return false
	default:
		return true
	}
}

// EnableVisualization turns on visualization with periodic updates
func (e *Executor) EnableVisualization(filename string, updateInterval time.Duration) {
	e.visualizeFile = filename
	
	// Start a goroutine to periodically update the visualization
	go func() {
		e.visualizeTicker = time.NewTicker(updateInterval)
		defer e.visualizeTicker.Stop()
		
		for {
			select {
			case <-e.finished:
				// Final visualization update
				e.updateVisualization()
				return
			case <-e.visualizeTicker.C:
				// Periodic visualization update
				e.updateVisualization()
			}
		}
	}()
}

// updateVisualization exports the current state to the visualization file
func (e *Executor) updateVisualization() {
	if e.visualizeFile == "" {
		return
	}
	
	// Determine file type and export accordingly
	if strings.HasSuffix(e.visualizeFile, ".json") {
		e.visualization.ExportToJSON(e.visualizeFile)
	} else if strings.HasSuffix(e.visualizeFile, ".dot") {
		e.visualization.ExportToDOT(e.visualizeFile)
	} else if strings.HasSuffix(e.visualizeFile, ".txt") {
		e.visualization.ExportToText(e.visualizeFile)
	}
}

// GetVisualization returns the visualization helper for manual export
func (e *Executor) GetVisualization() *DAGVisualization {
	return e.visualization
}