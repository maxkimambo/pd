# Migration Plan: Transitioning from internal/dag to taskmanager/workflow

## Executive Summary
This document outlines a phased approach to migrate from the current `internal/dag` implementation to the new `taskmanager/workflow` implementation while maintaining system stability and functionality.

## Current Architecture

### Current Flow (Using internal/dag)
```
cmd/disk_cmd.go & cmd/compute_cmd.go
    ↓
orchestrator.NewDAGOrchestrator()
    ↓
internal/dag (parallel execution engine)
    ↓
migrator/tasks/* (actual task implementations)
```

### New Architecture (Using taskmanager)
```
cmd/disk_cmd.go & cmd/compute_cmd.go
    ↓
workflow.NewMigrationManager()
    ↓
internal/taskmanager (currently sequential only)
    ↓
migrator/tasks/* (same task implementations)
```

## Phase 1: Feature Parity Analysis

### Missing Features in taskmanager
1. **Parallel Execution** (CRITICAL)
   - Current: dag.Executor supports concurrent task execution
   - Required: Add parallel execution to taskmanager.Workflow

2. **Task Timeouts**
   - Current: dag.Executor has configurable timeouts
   - Required: Add timeout support to taskmanager

3. **Concurrency Limits**
   - Current: dag.Executor.config.MaxParallelTasks
   - Required: Add semaphore/worker pool to taskmanager

4. **Progress Tracking**
   - Current: dag provides detailed status (Pending, Running, Completed, Failed)
   - Required: Enhance taskmanager status tracking

5. **Cancellation Support**
   - Current: dag.Executor supports context cancellation
   - Required: Ensure proper context propagation in taskmanager

## Phase 2: Implementation Tasks

### 2.1 Enhance taskmanager Package
```go
// Add to taskmanager/workflow.go
type ExecutionConfig struct {
    MaxParallelTasks int
    TaskTimeout      time.Duration
    ProgressInterval time.Duration
}

// Modify Execute to support parallel execution
func (w *Workflow) ExecuteParallel(ctx context.Context, config ExecutionConfig) error {
    // Implementation needed
}
```

### 2.2 Create Feature Toggle
```go
// Add to cmd/root.go
var useNewWorkflow bool

func init() {
    rootCmd.PersistentFlags().BoolVar(&useNewWorkflow, "use-new-workflow", false, 
        "Use new taskmanager-based workflow engine (experimental)")
}
```

### 2.3 Update CMD Layer
```go
// Modify cmd/disk_cmd.go
func runConvert(cmd *cobra.Command, args []string) error {
    // ... existing setup ...
    
    if useNewWorkflow {
        // New path
        migrationManager := workflow.NewMigrationManager(gcpClient, &config)
        result, err := migrationManager.ExecuteDiskMigrations(ctx, discoveredDisks)
    } else {
        // Current path
        taskOrchestrator := orchestrator.NewDAGOrchestrator(&config, gcpClient)
        result, err := taskOrchestrator.ExecuteDiskMigrations(ctx, discoveredDisks)
    }
}
```

### 2.4 Implement Missing Methods in MigrationManager
The workflow.MigrationManager needs these methods to match orchestrator.DAGOrchestrator:
- `ExecuteDiskMigrations(ctx, disks) (result, error)`
- `ExecuteInstanceMigrations(ctx, instances) (result, error)`
- `ProcessExecutionResults(result) error`

### 2.5 Enhance State Management
Replace generic map with type-safe context:
```go
// In taskmanager/context.go
type SharedContextData struct {
    GCPClient        *gcp.Clients
    Config           *migrator.Config
    MigrationResults []migrator.MigrationResult
    DetachedDisks    map[string]map[string]interface{}
    MigratedDisks    map[string]string
    FailedMigrations map[string]string
    // ... other fields
}
```

### 2.6 Implement Rollback Mechanism
Add rollback support to tasks:
```go
// In taskmanager/task.go
type Task struct {
    ID        string
    Handler   TaskFunc
    Rollback  TaskFunc // Optional rollback function
    DependsOn []string
}
```

## Phase 3: Testing Strategy

### 3.1 Unit Tests
- [ ] Test parallel execution in taskmanager
- [ ] Test timeout handling
- [ ] Test cancellation
- [ ] Test progress reporting

### 3.2 Integration Tests
- [ ] Create tests that run same workflow with both engines
- [ ] Compare results for correctness
- [ ] Benchmark performance differences

### 3.3 A/B Testing
```bash
# Test with old engine
pd migrate disk --project test-project --zone us-central1-a

# Test with new engine
pd migrate disk --project test-project --zone us-central1-a --use-new-workflow
```

## Phase 4: Migration Steps

### Step 1: Implement Core Features (Week 1-3)
- Add parallel execution to taskmanager
- Add timeout support
- Add progress tracking
- Implement type-safe SharedContextData
- Add rollback mechanism

### Step 2: Integration (Week 4)
- Implement feature toggle
- Update cmd layer to support both paths
- Complete MigrationManager implementation

### Step 3: Testing & Validation (Week 5-7)
- Run comprehensive tests
- Performance benchmarking
- Fix any issues discovered
- Implement detailed comparison logging

### Step 4: Gradual Rollout (Week 8-10)
- Enable for internal testing with flag
- Run both engines in parallel (dry-run mode)
- Monitor for behavioral differences
- Gradually increase usage

### Step 5: Cutover (Week 11-13)
- Make new workflow the default
- Keep old implementation with flag to disable
- Monitor production usage

### Step 6: Cleanup (Week 14-15)
- Remove old dag implementation
- Remove feature toggle
- Update documentation

## Implementation Checklist

### Immediate Tasks
- [ ] Add ExecuteParallel method to taskmanager.Workflow
- [ ] Implement worker pool for concurrent execution
- [ ] Add timeout support to task execution
- [ ] Add progress callback mechanism

### Integration Tasks
- [ ] Add feature toggle to cmd package
- [ ] Implement ExecuteDiskMigrations in MigrationManager
- [ ] Implement ExecuteInstanceMigrations in MigrationManager
- [ ] Implement ProcessExecutionResults in MigrationManager
- [ ] Update all imports and dependencies

### Testing Tasks
- [ ] Create comparison tests between engines
- [ ] Add performance benchmarks
- [ ] Test error handling scenarios
- [ ] Test cancellation and cleanup

## Risk Mitigation

1. **Performance Regression**
   - Mitigation: Benchmark before switching
   - Fallback: Keep feature toggle for quick rollback

2. **Missing Edge Cases**
   - Mitigation: Extensive testing with both engines
   - Fallback: Run both engines in parallel during transition

3. **API Incompatibility**
   - Mitigation: Ensure MigrationManager matches DAGOrchestrator interface
   - Fallback: Adapter pattern if needed

4. **Subtle Behavioral Differences**
   - Mitigation: Detailed logging during parallel-run phase
   - Fallback: Compare execution traces between engines
   - Prevention: Implement comprehensive behavioral tests

5. **Rollback Failures**
   - Mitigation: Test rollback scenarios thoroughly
   - Fallback: Manual intervention procedures documented
   - Prevention: Ensure all tasks are idempotent

## Success Criteria

1. New engine passes all existing tests
2. Performance is within 10% of current implementation
3. No increase in error rates during migration
4. Successful production runs for 2 weeks

## Notes for Implementation

### Key Differences to Address
1. **State Management**: dag uses stateful Task objects, taskmanager uses stateless functions
2. **Error Handling**: Ensure consistent error propagation
3. **Logging**: Maintain same log detail level
4. **Metrics**: Ensure all metrics are captured

### Code Mapping
```
dag.NewDAG() → taskmanager.NewWorkflowBuilder()
dag.Node → Not needed (tasks are added directly)
dag.Task → taskmanager.TaskFunc
dag.Executor → taskmanager.Workflow.Execute()
dag.ExecutionResult → Need to create similar structure
```

## Appendix: Sample Implementation

### Parallel Execution for taskmanager
```go
func (w *Workflow) ExecuteParallel(ctx context.Context, config ExecutionConfig) error {
    sorted, err := w.TopologicalSort()
    if err != nil {
        return err
    }
    
    // Group tasks by dependency level
    levels := w.groupByDependencyLevel(sorted)
    
    // Execute each level in parallel
    for _, level := range levels {
        if err := w.executeLevelParallel(ctx, level, config); err != nil {
            // Trigger rollback for completed tasks
            w.rollbackCompletedTasks(ctx)
            return err
        }
    }
    
    return nil
}

// Worker pool implementation for future optimization
func (w *Workflow) ExecuteWithWorkerPool(ctx context.Context, config ExecutionConfig) error {
    readyQueue := make(chan *Task, len(w.Tasks))
    workerPool := make(chan struct{}, config.MaxParallelTasks)
    
    // As tasks complete, check for newly ready tasks
    // and add them to the queue dynamically
    // (Implementation details omitted for brevity)
}
```

### Type-Safe Context Implementation
```go
// taskmanager/context.go
type SharedContext struct {
    mu   sync.RWMutex
    data *SharedContextData
}

func (sc *SharedContext) GetGCPClient() *gcp.Clients {
    sc.mu.RLock()
    defer sc.mu.RUnlock()
    return sc.data.GCPClient
}

func (sc *SharedContext) SetMigrationResult(result migrator.MigrationResult) {
    sc.mu.Lock()
    defer sc.mu.Unlock()
    sc.data.MigrationResults = append(sc.data.MigrationResults, result)
}
```

## Additional Considerations

### Observability Requirements
- Task execution time per task type
- Success/failure rates
- Resource utilization (API quota consumption)
- Rollback frequency and success rate

### Idempotency Guidelines
- All tasks must be safe to retry
- Use resource labels/tags to track migration state
- Implement "check before create" patterns
- Use deterministic naming for created resources

This plan provides a structured approach to migrate from internal/dag to taskmanager while minimizing risk and ensuring system stability.