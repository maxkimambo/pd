package taskmanager

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

type TaskResult struct {
	TaskID  string
	Result  interface{} // Result of the task execution, can be any type
	Success bool
	Error   error
	Status  string // e.g., "pending", "scheduled", "running", "completed", "failed"
}

type Task struct {
	id     string
	name   string
	status string
	start  time.Time
	end    time.Time
	exec   func() (TaskResult, error) // Execute runs the task and returns the result.
}

func (t *Task) ID() string {
	return t.id
}
func (t *Task) Name() string {
	return t.name
}
func (t *Task) Start() {
	t.start = time.Now()
	t.status = "running"
}
func (t *Task) Finish() {
	t.end = time.Now()
	t.status = "completed"
	duration := t.end.Sub(t.start)
	fmt.Println("Task", t.id, "finished in", duration, "status:", t.status)
}

func (t *Task) Abort() {
	t.end = time.Now()
	t.status = "aborted"
}
func (t *Task) Status() string {
	return t.status
}
func (t *Task) Execute() (TaskResult, error) {
	if t.exec == nil {
		return TaskResult{}, fmt.Errorf("no execution function defined for task %s", t.id)
	}
	result, err := t.exec()
	if err != nil {
		return TaskResult{}, err
	}
	return result, nil
}

func NewTask(id, name string, exec func() (TaskResult, error)) *Task {
	return &Task{
		id:     id,
		name:   name,
		status: "pending",
		start:  time.Time{},
		end:    time.Time{},
		exec:   exec,
	}
}

type BaseTask interface {
	ID() string
	Name() string
	Execute() (TaskResult, error)
	Status() string
	Start()
	Finish()
	Abort()
}
type Node struct {
	ID      string
	Name    string
	Type    string
	Task    BaseTask                   // Task represent a single task in a process.
	Execute func() (TaskResult, error) // Execute runs the task and returns the result.
}

func NewNode(id, name, nodeType string, task BaseTask) *Node {
	return &Node{
		ID:      id,
		Name:    name,
		Task:    task,
		Execute: task.Execute,
	}
}

func NewDAG() *DAG {
	return &DAG{
		Nodes: make(map[string]*Node),
		Edges: make(map[string][]string),
	}
}

type DAG struct {
	mu               sync.RWMutex
	Roots            []*Node // Nodes that have no dependencies
	Nodes            map[string]*Node
	Edges            map[string][]string // maps from node ID to list of dependent node IDs
	indegrees        map[string]int      // maps from node ID to its indegree count
	isFinalized      bool                // Indicates if the DAG has been finalized, no further modifications allowed
	taskOrder        []string            // The order of tasks after topological sort
	currentTaskIndex int                 // Index of the current task being executed in the taskOrder
}

func (d *DAG) AddNode(node *Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isFinalized {
		return fmt.Errorf("DAG is finalized, no further modifications allowed")
	}
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if _, exists := d.Nodes[node.ID]; exists {
		return fmt.Errorf("node with ID %s already exists", node.ID)
	}

	d.Nodes[node.ID] = node

	return nil
}

func (d *DAG) topologicalSort() ([]string, error) {
	// d.mu.RLock()
	// defer d.mu.RUnlock()
	taskOrder := []string{}
	// init indegrees to 0
	indegrees := make(map[string]int)
	for k := range d.indegrees {
		indegrees[k] = 0
	}
	// calculate indegress
	for fromID, toIDs := range d.Edges {
		indegrees[fromID] = 0 // Ensure source node has indegree 0
		for _, toID := range toIDs {
			// A -> B means A:[B,C...] we only count the number of times B is a target node
			indegrees[toID]++
		}
	}

	// get root nodes (indegree 0)
	queue := []string{}
	for id, indegree := range indegrees {
		if indegree == 0 {
			queue = append(queue, id)
		}
	}
	// do a bfs traversal to get the order of nodes
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:] // remove it from the queue
		taskOrder = append(taskOrder, node)
		// Get all dependents (outgoing edges)
		for _, dep := range d.Edges[node] {
			indegrees[dep]--
			if indegrees[dep] == 0 {
				queue = append(queue, dep) // If indegree becomes 0, add to queue
			}
		}
	}

	if len(taskOrder) != len(d.Nodes) {
		return nil, fmt.Errorf("DAG has a cycle or is incomplete, task order cannot be determined")
	}

	return taskOrder, nil
}
func (d *DAG) Finalize() {
	// Finalize the DAG by calculating indegrees and identifying root nodes
	d.mu.Lock()
	defer d.mu.Unlock()
	orderedTaskIds, err := d.topologicalSort()
	if err != nil {
		panic(fmt.Sprintf("failed to finalize DAG: %v", err))
	}
	d.taskOrder = orderedTaskIds
	d.isFinalized = true
}

func (d *DAG) PullNextTask() (*Node, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isFinalized && d.currentTaskIndex >= len(d.taskOrder) {
		return nil, fmt.Errorf("no more tasks to execute")
	}
	if !d.isFinalized {
		return nil, fmt.Errorf("DAG is not finalized, cannot pull next task")
	}

	if d.currentTaskIndex < len(d.taskOrder) {
		nodeID := d.taskOrder[d.currentTaskIndex]
		d.currentTaskIndex++
		return d.Nodes[nodeID], nil
	}
	return nil, fmt.Errorf("no more tasks to execute")
}

func (d *DAG) AddDependency(fromID, toID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isFinalized {
		return fmt.Errorf("DAG is finalized, no further modifications allowed")
	}
	if _, exists := d.Nodes[fromID]; !exists {
		return fmt.Errorf("source node %s does not exist", fromID)
	}
	if _, exists := d.Nodes[toID]; !exists {
		return fmt.Errorf("target node %s does not exist", toID)
	}
	if fromID == toID {
		return fmt.Errorf("cannot add dependency from a node to itself: %s", fromID)
	}
	// ensure no duplicate dependencies
	if slices.Contains(d.Edges[fromID], toID) {
		return fmt.Errorf("duplicate dependency found from %s to %s", fromID, toID)
	}

	d.Edges[fromID] = append(d.Edges[fromID], toID)

	if d.createsACycle(fromID, toID) {
		return fmt.Errorf("adding dependency from %s to %s would create a cycle", fromID, toID)
	}

	return nil
}

func (d *DAG) GetDependencies(nodeID string) ([]string, error) {
	if _, exists := d.Nodes[nodeID]; !exists {
		return nil, fmt.Errorf("node %s does not exist", nodeID)
	}
	return d.Edges[nodeID], nil
}

func (d *DAG) GetNode(nodeID string) (*Node, error) {
	node, exists := d.Nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s does not exist", nodeID)
	}
	return node, nil
}

func (d *DAG) hasCycle() bool {
	// If there are no nodes, there can't be a cycle
	if len(d.Nodes) == 0 {
		return false
	}
	// Check for cycles using Kahn's algorithm (BFS approach)
	indegrees := make(map[string]int)

	// Initialize indegrees for all nodes
	for id := range d.Nodes {
		indegrees[id] = 0
	}

	// count indegrees based on current edges
	for _, toIDs := range d.Edges {
		for _, toID := range toIDs {
			if _, exists := d.Nodes[toID]; exists {
				indegrees[toID]++
			}
		}
	}

	queue := []string{}
	// collect nodes with indegree 0 and add to queue
	for id, indegree := range indegrees {
		if indegree == 0 {
			queue = append(queue, id)
		}
	}
	// If no nodes with indegree 0, cycle exists
	if len(queue) == 0 {
		return true
	}
	// bfs, if we can visit all nodes, then no cycle exists
	visited := 0

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		// Get all dependents (outgoing edges)
		for _, dep := range d.Edges[node] {
			indegrees[dep]-- // Decrease indegree for each dependent
			if indegrees[dep] == 0 {
				queue = append(queue, dep) // If indegree becomes 0, add to queue
			}
		}
	}
	// If visited nodes count is not equal to total nodes, cycle exists
	return visited != len(d.Nodes)
}

func (d *DAG) createsACycle(fromNode, toNode string) bool {
	// Logic is if we can reach toNode starting from fromNode
	// we do have a cycle by adding an edge from fromNode to toNode
	// this is because the graph is directional.

	visited := make(map[string]bool)
	var dfs func(string) bool
	dfs = func(currNode string) bool {
		if currNode == fromNode {
			return true // Found the source node, cycle would be created if edge is added
		}
		if visited[currNode] {
			return true // Cycle detected
		}
		visited[currNode] = true
		for _, neighbour := range d.Edges[currNode] {
			if dfs(neighbour) {
				return true // Cycle detected in the recursive call
			}
		}
		return false
	}
	return dfs(toNode)
}

// func (d *DAG) detectCycle(nodeID string, visited, recStack map[string]bool) bool {

// 	if recStack[nodeID] {
// 		return true // Cycle detected
// 	}

// 	if visited[nodeID] {
// 		return false // Already visited
// 	}

// 	visited[nodeID] = true
// 	recStack[nodeID] = true

// 	for _, neighbor := range d.Edges[nodeID] {
// 		if d.detectCycle(neighbor, visited, recStack) {
// 			return true
// 		}
// 	}

// 	recStack[nodeID] = false
// 	return false
// }
