package dag

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// DAGVisualization provides tools to visualize and monitor DAG execution
type DAGVisualization struct {
	dag *DAG
}

// NewDAGVisualization creates a new visualization helper
func NewDAGVisualization(dag *DAG) *DAGVisualization {
	return &DAGVisualization{
		dag: dag,
	}
}

// NodeInfo contains information about a node for visualization
type NodeInfo struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Status      NodeStatus `json:"status"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Duration    string     `json:"duration,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// EdgeInfo contains information about an edge for visualization
type EdgeInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DAGInfo contains the full DAG structure for visualization
type DAGInfo struct {
	Nodes []NodeInfo `json:"nodes"`
	Edges []EdgeInfo `json:"edges"`
	Stats DAGStats   `json:"stats"`
}

// DAGStats contains statistics about the DAG execution
type DAGStats struct {
	TotalNodes    int           `json:"totalNodes"`
	CompletedNodes int          `json:"completedNodes"`
	FailedNodes   int           `json:"failedNodes"`
	RunningNodes  int           `json:"runningNodes"`
	PendingNodes  int           `json:"pendingNodes"`
	TotalDuration string        `json:"totalDuration,omitempty"`
	StartTime     *time.Time    `json:"startTime,omitempty"`
	EndTime       *time.Time    `json:"endTime,omitempty"`
}

// GenerateDAGInfo creates a representation of the DAG for visualization
func (v *DAGVisualization) GenerateDAGInfo() (*DAGInfo, error) {
	// Get all nodes
	nodeIDs := v.dag.GetAllNodes()
	
	// Create node info
	nodes := make([]NodeInfo, 0, len(nodeIDs))
	stats := DAGStats{
		TotalNodes: len(nodeIDs),
	}
	
	var earliestStart, latestEnd *time.Time
	
	for _, id := range nodeIDs {
		node, exists := v.dag.nodes[id]
		if !exists {
			continue
		}
		
		task := node.GetTask()
		status := node.GetStatus()
		startTime := node.GetStartTime()
		endTime := node.GetEndTime()
		
		// Calculate duration
		duration := ""
		if startTime != nil && endTime != nil {
			duration = endTime.Sub(*startTime).String()
		} else if startTime != nil {
			duration = time.Since(*startTime).String() + " (running)"
		}
		
		// Track earliest start and latest end for total duration
		if startTime != nil {
			if earliestStart == nil || startTime.Before(*earliestStart) {
				earliestStart = startTime
			}
		}
		if endTime != nil {
			if latestEnd == nil || endTime.After(*latestEnd) {
				latestEnd = endTime
			}
		}
		
		// Get error message if node failed
		errorMsg := ""
		if status == StatusFailed {
			if nodeErr := node.GetError(); nodeErr != nil {
				errorMsg = nodeErr.Error()
			}
		}
		
		nodeInfo := NodeInfo{
			ID:          id,
			Type:        GetTaskType(task),
			Description: GetTaskDescription(task),
			Status:      status,
			StartTime:   startTime,
			EndTime:     endTime,
			Duration:    duration,
			Error:       errorMsg,
		}
		
		nodes = append(nodes, nodeInfo)
		
		// Update stats
		switch status {
		case StatusCompleted:
			stats.CompletedNodes++
		case StatusFailed:
			stats.FailedNodes++
		case StatusRunning:
			stats.RunningNodes++
		case StatusPending:
			stats.PendingNodes++
		}
	}
	
	// Calculate total duration if we have start and end times
	if earliestStart != nil {
		stats.StartTime = earliestStart
		if latestEnd != nil {
			stats.EndTime = latestEnd
			stats.TotalDuration = latestEnd.Sub(*earliestStart).String()
		} else {
			stats.TotalDuration = time.Since(*earliestStart).String() + " (running)"
		}
	}
	
	// Create edge info
	edges := []EdgeInfo{}
	for _, fromID := range nodeIDs {
		dependents, err := v.dag.GetDependents(fromID)
		if err != nil {
			continue // Skip nodes with dependency errors
		}
		
		for _, toID := range dependents {
			edges = append(edges, EdgeInfo{
				From: fromID,
				To:   toID,
			})
		}
	}
	
	return &DAGInfo{
		Nodes: nodes,
		Edges: edges,
		Stats: stats,
	}, nil
}

// ExportToJSON exports the DAG visualization to a JSON file
func (v *DAGVisualization) ExportToJSON(filename string) error {
	dagInfo, err := v.GenerateDAGInfo()
	if err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(dagInfo, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, data, 0644)
}

// GenerateDOTGraph creates a DOT format graph for visualization with Graphviz
func (v *DAGVisualization) GenerateDOTGraph() (string, error) {
	dagInfo, err := v.GenerateDAGInfo()
	if err != nil {
		return "", err
	}
	
	// Create DOT graph
	var sb strings.Builder
	sb.WriteString("digraph MigrationDAG {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=filled];\n")
	sb.WriteString("  label=\"Migration DAG Execution Status\";\n")
	sb.WriteString("  labelloc=\"t\";\n\n")
	
	// Add legend
	sb.WriteString("  // Legend\n")
	sb.WriteString("  subgraph cluster_legend {\n")
	sb.WriteString("    label=\"Status Legend\";\n")
	sb.WriteString("    style=filled;\n")
	sb.WriteString("    fillcolor=lightgrey;\n")
	sb.WriteString("    \"legend_pending\" [label=\"Pending\", fillcolor=\"lightgrey\"];\n")
	sb.WriteString("    \"legend_running\" [label=\"Running\", fillcolor=\"lightblue\"];\n")
	sb.WriteString("    \"legend_completed\" [label=\"Completed\", fillcolor=\"lightgreen\"];\n")
	sb.WriteString("    \"legend_failed\" [label=\"Failed\", fillcolor=\"salmon\"];\n")
	sb.WriteString("    \"legend_cancelled\" [label=\"Cancelled\", fillcolor=\"orange\"];\n")
	sb.WriteString("    \"legend_pending\" -> \"legend_running\" -> \"legend_completed\" [style=invis];\n")
	sb.WriteString("  }\n\n")
	
	// Add nodes with status-based colors
	for _, node := range dagInfo.Nodes {
		color := "white"
		switch node.Status {
		case StatusPending:
			color = "lightgrey"
		case StatusRunning:
			color = "lightblue"
		case StatusCompleted:
			color = "lightgreen"
		case StatusFailed:
			color = "salmon"
		case StatusCancelled:
			color = "orange"
		}
		
		// Create multi-line label with node info
		label := fmt.Sprintf("%s\\n%s", node.ID, node.Type)
		if node.Duration != "" {
			label += fmt.Sprintf("\\n%s", node.Duration)
		}
		if node.Error != "" {
			// Truncate long error messages
			errorMsg := node.Error
			if len(errorMsg) > 50 {
				errorMsg = errorMsg[:47] + "..."
			}
			label += fmt.Sprintf("\\nError: %s", errorMsg)
		}
		
		sb.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", fillcolor=\"%s\"];\n", 
			node.ID, label, color))
	}
	
	sb.WriteString("\n")
	
	// Add edges
	for _, edge := range dagInfo.Edges {
		sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", edge.From, edge.To))
	}
	
	// Add stats as a separate node (always include, even for empty DAG)
	sb.WriteString("\n  // Statistics\n")
	statsLabel := fmt.Sprintf("Stats\\nTotal: %d\\nCompleted: %d\\nFailed: %d\\nRunning: %d\\nPending: %d",
		dagInfo.Stats.TotalNodes,
		dagInfo.Stats.CompletedNodes,
		dagInfo.Stats.FailedNodes,
		dagInfo.Stats.RunningNodes,
		dagInfo.Stats.PendingNodes)
	
	if dagInfo.Stats.TotalDuration != "" {
		statsLabel += fmt.Sprintf("\\nDuration: %s", dagInfo.Stats.TotalDuration)
	}
	
	sb.WriteString(fmt.Sprintf("  \"stats\" [label=\"%s\", shape=note, fillcolor=\"lightyellow\"];\n", statsLabel))
	
	sb.WriteString("}\n")
	
	return sb.String(), nil
}

// ExportToDOT exports the DAG visualization to a DOT file
func (v *DAGVisualization) ExportToDOT(filename string) error {
	dot, err := v.GenerateDOTGraph()
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, []byte(dot), 0644)
}

// GenerateTextSummary creates a human-readable text summary of the DAG execution
func (v *DAGVisualization) GenerateTextSummary() (string, error) {
	dagInfo, err := v.GenerateDAGInfo()
	if err != nil {
		return "", err
	}
	
	var sb strings.Builder
	
	// Header
	sb.WriteString("=== Migration DAG Execution Summary ===\n\n")
	
	// Overall stats
	sb.WriteString("Overall Statistics:\n")
	sb.WriteString(fmt.Sprintf("  Total Nodes: %d\n", dagInfo.Stats.TotalNodes))
	sb.WriteString(fmt.Sprintf("  Completed: %d\n", dagInfo.Stats.CompletedNodes))
	sb.WriteString(fmt.Sprintf("  Failed: %d\n", dagInfo.Stats.FailedNodes))
	sb.WriteString(fmt.Sprintf("  Running: %d\n", dagInfo.Stats.RunningNodes))
	sb.WriteString(fmt.Sprintf("  Pending: %d\n", dagInfo.Stats.PendingNodes))
	
	if dagInfo.Stats.TotalDuration != "" {
		sb.WriteString(fmt.Sprintf("  Total Duration: %s\n", dagInfo.Stats.TotalDuration))
	}
	
	if dagInfo.Stats.StartTime != nil {
		sb.WriteString(fmt.Sprintf("  Start Time: %s\n", dagInfo.Stats.StartTime.Format(time.RFC3339)))
	}
	
	if dagInfo.Stats.EndTime != nil {
		sb.WriteString(fmt.Sprintf("  End Time: %s\n", dagInfo.Stats.EndTime.Format(time.RFC3339)))
	}
	
	// Progress percentage
	if dagInfo.Stats.TotalNodes > 0 {
		progress := float64(dagInfo.Stats.CompletedNodes) / float64(dagInfo.Stats.TotalNodes) * 100
		sb.WriteString(fmt.Sprintf("  Progress: %.1f%%\n", progress))
	}
	
	sb.WriteString("\n")
	
	// Node details by status
	statusGroups := map[string][]NodeInfo{
		"Failed":    {},
		"Running":   {},
		"Completed": {},
		"Pending":   {},
	}
	
	for _, node := range dagInfo.Nodes {
		switch node.Status {
		case StatusFailed:
			statusGroups["Failed"] = append(statusGroups["Failed"], node)
		case StatusRunning:
			statusGroups["Running"] = append(statusGroups["Running"], node)
		case StatusCompleted:
			statusGroups["Completed"] = append(statusGroups["Completed"], node)
		case StatusPending:
			statusGroups["Pending"] = append(statusGroups["Pending"], node)
		}
	}
	
	// Show details for each status group
	for status, nodes := range statusGroups {
		if len(nodes) > 0 {
			sb.WriteString(fmt.Sprintf("%s Nodes (%d):\n", status, len(nodes)))
			for _, node := range nodes {
				sb.WriteString(fmt.Sprintf("  - %s (%s)", node.ID, node.Type))
				if node.Duration != "" {
					sb.WriteString(fmt.Sprintf(" - %s", node.Duration))
				}
				if node.Error != "" {
					sb.WriteString(fmt.Sprintf(" - Error: %s", node.Error))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}
	
	return sb.String(), nil
}

// ExportToText exports the DAG summary to a text file
func (v *DAGVisualization) ExportToText(filename string) error {
	summary, err := v.GenerateTextSummary()
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, []byte(summary), 0644)
}