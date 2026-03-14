package components

import (
	"time"

	pcontext "github.com/rhystic/attractor/pkg/context"
)

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
