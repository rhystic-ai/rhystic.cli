package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	pcontext "github.com/rhystic/attractor/pkg/context"
)

// DefaultLogsRoot is the default location for Attractor logs
const DefaultLogsRoot = "./attractor-logs"

// RunSummary is a lightweight view of a run for the list view
type RunSummary struct {
	RunID        string
	PipelineName string
	Goal         string
	Status       string
	CurrentNode  string
	CompletedAt  time.Time
	NodeCount    int
	CurrentNodes []string
}

// RunDetail is the full run data for detail view
type RunDetail struct {
	RunSummary
	Checkpoint   *pcontext.Checkpoint
	NodeOutcomes map[string]pcontext.Outcome
	Nodes        []NodeDetail
}

// NodeDetail contains all data for a single node execution
type NodeDetail struct {
	ID             string
	Label          string
	Prompt         string
	Response       string
	Status         string
	Notes          string
	FailureReason  string
	ContextUpdates map[string]any
	CompletedAt    time.Time
}

// LoadRuns scans the logs directory and returns summaries of all runs
func LoadRuns(logsRoot string) ([]RunSummary, error) {
	if logsRoot == "" {
		logsRoot = DefaultLogsRoot
	}

	entries, err := os.ReadDir(logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunSummary{}, nil
		}
		return nil, fmt.Errorf("read logs directory: %w", err)
	}

	var runs []RunSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runID := entry.Name()
		summary, err := loadRunSummary(logsRoot, runID)
		if err != nil {
			// Skip runs that can't be loaded
			continue
		}

		runs = append(runs, *summary)
	}

	// Sort by timestamp, newest first
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CompletedAt.After(runs[j].CompletedAt)
	})

	return runs, nil
}

// loadRunSummary loads just the summary data for a run
func loadRunSummary(logsRoot, runID string) (*RunSummary, error) {
	checkpoint, err := LoadCheckpoint(logsRoot, runID)
	if err != nil {
		return nil, err
	}

	summary := &RunSummary{
		RunID:        runID,
		CurrentNode:  checkpoint.CurrentNodeID,
		CompletedAt:  checkpoint.CreatedAt,
		NodeCount:    len(checkpoint.CompletedNodes),
		CurrentNodes: checkpoint.CompletedNodes,
	}

	// Extract pipeline name and goal from context
	if checkpoint.Context != nil {
		if v, ok := checkpoint.Context.Get("graph.label"); ok {
			summary.PipelineName = fmt.Sprintf("%v", v)
		}
		if v, ok := checkpoint.Context.Get("goal"); ok {
			summary.Goal = fmt.Sprintf("%v", v)
		}
	}

	// Determine overall status from last node outcome
	if len(checkpoint.CompletedNodes) > 0 {
		lastNode := checkpoint.CompletedNodes[len(checkpoint.CompletedNodes)-1]
		if outcome, ok := checkpoint.NodeOutcomes[lastNode]; ok {
			summary.Status = string(outcome.Status)
		}
	}

	if summary.Status == "" {
		summary.Status = "unknown"
	}

	return summary, nil
}

// LoadRun loads full details for a single run
func LoadRun(logsRoot, runID string) (*RunDetail, error) {
	if logsRoot == "" {
		logsRoot = DefaultLogsRoot
	}

	summary, err := loadRunSummary(logsRoot, runID)
	if err != nil {
		return nil, err
	}

	checkpoint, err := LoadCheckpoint(logsRoot, runID)
	if err != nil {
		return nil, err
	}

	detail := &RunDetail{
		RunSummary:   *summary,
		Checkpoint:   checkpoint,
		NodeOutcomes: checkpoint.NodeOutcomes,
	}

	// Load node details
	for _, nodeID := range checkpoint.CompletedNodes {
		node, err := LoadNode(logsRoot, runID, nodeID)
		if err != nil {
			// Create minimal node info if loading fails
			node = &NodeDetail{
				ID:     nodeID,
				Status: "unknown",
			}
		}
		detail.Nodes = append(detail.Nodes, *node)
	}

	return detail, nil
}

// LoadNode loads prompt, response, and status for a node
func LoadNode(logsRoot, runID, nodeID string) (*NodeDetail, error) {
	stageDir := filepath.Join(logsRoot, runID, nodeID)

	node := &NodeDetail{
		ID: nodeID,
	}

	// Try to load status file first
	status, err := pcontext.ReadStatus(stageDir)
	if err == nil {
		node.Status = string(status.Status)
		node.Notes = status.Notes
		node.FailureReason = status.FailureReason
		node.ContextUpdates = status.ContextUpdates
	}

	// Load prompt
	if data, err := os.ReadFile(filepath.Join(stageDir, "prompt.md")); err == nil {
		node.Prompt = string(data)
	}

	// Load response
	if data, err := os.ReadFile(filepath.Join(stageDir, "response.md")); err == nil {
		node.Response = string(data)
	}

	return node, nil
}

// LoadCheckpoint loads the checkpoint.json for a run
func LoadCheckpoint(logsRoot, runID string) (*pcontext.Checkpoint, error) {
	path := filepath.Join(logsRoot, runID, "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var cp pcontext.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	return &cp, nil
}

// GetRunCount returns the number of runs in the logs directory
func GetRunCount(logsRoot string) (int, error) {
	if logsRoot == "" {
		logsRoot = DefaultLogsRoot
	}

	entries, err := os.ReadDir(logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}

	return count, nil
}
