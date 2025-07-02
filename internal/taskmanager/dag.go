package taskmanager

import "fmt"

// DAG represents a directed acyclic graph of nodes and edges.
type DAG struct {
	nodes    map[string]bool
	edges    map[string][]string // node -> list of nodes it depends on
	inDegree map[string]int      // node -> number of incoming edges
}

// NewDAG creates a new empty DAG.
func NewDAG() *DAG {
	return &DAG{
		nodes:    make(map[string]bool),
		edges:    make(map[string][]string),
		inDegree: make(map[string]int),
	}
}

// AddNode adds a node to the DAG.
func (d *DAG) AddNode(id string) {
	if !d.nodes[id] {
		d.nodes[id] = true
		d.inDegree[id] = 0
	}
}

// AddEdge adds a dependency edge from 'from' to 'to' (from depends on to).
func (d *DAG) AddEdge(from, to string) {
	// Ensure both nodes exist
	d.AddNode(from)
	d.AddNode(to)

	// Add dependency
	d.edges[from] = append(d.edges[from], to)
	d.inDegree[from]++
}

// TopologicalSort performs a topological sort and returns the nodes in execution order.
// Returns an error if a cycle is detected.
func (d *DAG) TopologicalSort() ([]string, error) {
	// Make a copy of inDegree since we'll modify it
	inDegree := make(map[string]int)
	for node, degree := range d.inDegree {
		inDegree[node] = degree
	}

	// Initialize queue with nodes that have no dependencies
	var queue []string
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	var result []string
	for len(queue) > 0 {
		// Remove first node from queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Find all nodes that depend on the current node and reduce their in-degree
		for node, deps := range d.edges {
			for _, depID := range deps {
				if depID == current {
					inDegree[node]--
					if inDegree[node] == 0 {
						queue = append(queue, node)
					}
				}
			}
		}
	}

	// Check for cycles
	if len(result) != len(d.nodes) {
		return nil, fmt.Errorf("circular dependency detected in DAG")
	}

	return result, nil
}
