package dag

import (
	"fmt"
	"sync"

	"github.com/autom8ter/dagger"
)

const (
	DefaultNodeType = "task"
	DefaultEdgeType = "dependency"
)

// DAG represents a directed acyclic graph for task execution
type DAG struct {
	graph *dagger.Graph
	nodes map[string]Node
	mutex sync.RWMutex
}

// NewDAG creates a new DAG instance
func NewDAG() *DAG {
	return &DAG{
		graph: dagger.NewGraph(),
		nodes: make(map[string]Node),
	}
}

// AddNode adds a task node to the DAG
func (d *DAG) AddNode(node Node) error {
	if node == nil {
		return fmt.Errorf("node cannot be nil")
	}
	
	id := node.ID()
	if id == "" {
		return fmt.Errorf("node ID cannot be empty")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// Check if node already exists
	if _, exists := d.nodes[id]; exists {
		return fmt.Errorf("node with ID %s already exists", id)
	}
	
	// Add to graph using dagger API
	path := dagger.Path{XID: id, XType: DefaultNodeType}
	attributes := dagger.Attributes{"node": node}
	d.graph.SetNode(path, attributes)
	
	d.nodes[id] = node
	return nil
}

// AddDependency creates a directed edge between nodes (from -> to)
func (d *DAG) AddDependency(fromID, toID string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// Verify both nodes exist
	if _, exists := d.nodes[fromID]; !exists {
		return fmt.Errorf("source node %s does not exist", fromID)
	}
	if _, exists := d.nodes[toID]; !exists {
		return fmt.Errorf("target node %s does not exist", toID)
	}
	
	// Add edge in graph
	fromPath := dagger.Path{XID: fromID, XType: DefaultNodeType}
	toPath := dagger.Path{XID: toID, XType: DefaultNodeType}
	edgeNode := dagger.Node{
		Path:       dagger.Path{XType: DefaultEdgeType},
		Attributes: dagger.Attributes{"type": "dependency"},
	}
	
	_, err := d.graph.SetEdge(fromPath, toPath, edgeNode)
	if err != nil {
		return fmt.Errorf("failed to add dependency from %s to %s: %w", fromID, toID, err)
	}
	
	return nil
}

// GetNode retrieves a node by its ID
func (d *DAG) GetNode(id string) (Node, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	node, exists := d.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node %s not found", id)
	}
	
	return node, nil
}

// GetDependencies returns all nodes that must complete before the given node can run
func (d *DAG) GetDependencies(id string) ([]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	if _, exists := d.nodes[id]; !exists {
		return nil, fmt.Errorf("node %s not found", id)
	}
	
	var deps []string
	nodePath := dagger.Path{XID: id, XType: DefaultNodeType}
	
	d.graph.RangeEdgesTo(DefaultEdgeType, nodePath, func(e dagger.Edge) bool {
		deps = append(deps, e.From.XID)
		return true
	})
	
	return deps, nil
}

// GetDependents returns all nodes that depend on the given node
func (d *DAG) GetDependents(id string) ([]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	if _, exists := d.nodes[id]; !exists {
		return nil, fmt.Errorf("node %s not found", id)
	}
	
	var dependents []string
	nodePath := dagger.Path{XID: id, XType: DefaultNodeType}
	
	d.graph.RangeEdgesFrom(DefaultEdgeType, nodePath, func(e dagger.Edge) bool {
		dependents = append(dependents, e.To.XID)
		return true
	})
	
	return dependents, nil
}

// GetAllNodes returns all node IDs in the DAG
func (d *DAG) GetAllNodes() []string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	nodeIDs := make([]string, 0, len(d.nodes))
	for id := range d.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	
	return nodeIDs
}

// GetRootNodes returns all nodes with no dependencies
func (d *DAG) GetRootNodes() ([]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	var roots []string
	for id := range d.nodes {
		deps, err := d.getDependenciesUnsafe(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get dependencies for %s: %w", id, err)
		}
		if len(deps) == 0 {
			roots = append(roots, id)
		}
	}
	
	return roots, nil
}

// getDependenciesUnsafe is an internal helper that doesn't acquire locks
func (d *DAG) getDependenciesUnsafe(id string) ([]string, error) {
	var deps []string
	nodePath := dagger.Path{XID: id, XType: DefaultNodeType}
	
	d.graph.RangeEdgesTo(DefaultEdgeType, nodePath, func(e dagger.Edge) bool {
		deps = append(deps, e.From.XID)
		return true
	})
	
	return deps, nil
}

// GetReadyNodes returns all nodes that are ready to execute (all dependencies completed)
func (d *DAG) GetReadyNodes() ([]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	var ready []string
	for id, node := range d.nodes {
		// Skip if already completed, failed, cancelled, or running
		status := node.GetStatus()
		if status == StatusCompleted || status == StatusFailed || status == StatusCancelled || status == StatusRunning {
			continue
		}
		
		// Check if all dependencies are completed
		deps, err := d.getDependenciesUnsafe(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get dependencies for %s: %w", id, err)
		}
		
		allDepsCompleted := true
		for _, depID := range deps {
			depNode := d.nodes[depID]
			if depNode.GetStatus() != StatusCompleted {
				allDepsCompleted = false
				break
			}
		}
		
		if allDepsCompleted {
			ready = append(ready, id)
		}
	}
	
	return ready, nil
}

// Validate checks if the DAG has cycles or other issues
func (d *DAG) Validate() error {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	// Check for cycles using topological sort
	// If we can't complete a topological sort, there's a cycle
	if d.hasCycle() {
		return fmt.Errorf("DAG contains cycles")
	}
	
	return nil
}

// hasCycle checks for cycles using DFS
func (d *DAG) hasCycle() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	
	for id := range d.nodes {
		if !visited[id] {
			if d.hasCycleDFS(id, visited, recStack) {
				return true
			}
		}
	}
	
	return false
}

// hasCycleDFS performs DFS to detect cycles
func (d *DAG) hasCycleDFS(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true
	
	// Get all dependents (outgoing edges)
	dependents, err := d.getDependentsUnsafe(nodeID)
	if err != nil {
		return false // If we can't get dependents, assume no cycle for this branch
	}
	
	for _, depID := range dependents {
		if !visited[depID] {
			if d.hasCycleDFS(depID, visited, recStack) {
				return true
			}
		} else if recStack[depID] {
			return true // Back edge found - cycle detected
		}
	}
	
	recStack[nodeID] = false
	return false
}

// getDependentsUnsafe is an internal helper that doesn't acquire locks
func (d *DAG) getDependentsUnsafe(id string) ([]string, error) {
	var dependents []string
	nodePath := dagger.Path{XID: id, XType: DefaultNodeType}
	
	d.graph.RangeEdgesFrom(DefaultEdgeType, nodePath, func(e dagger.Edge) bool {
		dependents = append(dependents, e.To.XID)
		return true
	})
	
	return dependents, nil
}

// Size returns the number of nodes in the DAG
func (d *DAG) Size() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return len(d.nodes)
}

// IsComplete returns true if all nodes have completed successfully
func (d *DAG) IsComplete() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	for _, node := range d.nodes {
		if node.GetStatus() != StatusCompleted {
			return false
		}
	}
	
	return true
}

// HasFailed returns true if any node has failed or been cancelled
func (d *DAG) HasFailed() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	for _, node := range d.nodes {
		status := node.GetStatus()
		if status == StatusFailed || status == StatusCancelled {
			return true
		}
	}
	
	return false
}

// Close closes the DAG and releases resources
func (d *DAG) Close() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	if d.graph != nil {
		d.graph.Close()
	}
}