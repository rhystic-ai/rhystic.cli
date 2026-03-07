// Package engine provides the pipeline execution engine.
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/handlers"
	"github.com/rhystic/attractor/pkg/llm"
	"github.com/rhystic/attractor/pkg/store"
)

// Config holds engine configuration.
type Config struct {
	LogsRoot             string // Directory for run logs
	MaxRetries           int    // Default max retries
	CheckpointAfter      bool   // Checkpoint after each node
	ResumeFromCheckpoint bool   // Resume from existing checkpoint
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		LogsRoot:        "./attractor-logs",
		MaxRetries:      50,
		CheckpointAfter: true,
	}
}

// Engine executes DOT-defined pipelines.
type Engine struct {
	Config   Config
	Graph    *dot.Graph
	Registry *handlers.Registry
	Events   *events.Emitter
	Context  *pcontext.Context
	Store    *store.Store // Optional persistence; nil disables DB writes.

	runID          string
	completedNodes []string
	nodeOutcomes   map[string]pcontext.Outcome
	totalUsage     llm.Usage // Cumulative usage across all nodes
}

// New creates a new engine for a graph.
func New(graph *dot.Graph, llmClient *llm.Client, cfg Config) *Engine {
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	return &Engine{
		Config:       cfg,
		Graph:        graph,
		Registry:     handlers.NewRegistry(llmClient),
		Events:       events.NewEmitter(runID),
		Context:      pcontext.New(),
		runID:        runID,
		nodeOutcomes: make(map[string]pcontext.Outcome),
	}
}

// Run executes the pipeline and returns the final outcome.
func (e *Engine) Run(ctx context.Context) (pcontext.Outcome, error) {
	// Subscribe to events to track usage
	eventCh := e.Events.Subscribe()
	go func() {
		for event := range eventCh {
			if event.Type == events.EventLLMEnd {
				e.totalUsage = e.totalUsage.Add(llm.Usage{
					InputTokens:  event.Data.InputTokens,
					OutputTokens: event.Data.OutputTokens,
					TotalTokens:  event.Data.InputTokens + event.Data.OutputTokens,
				})
			}
		}
	}()

	// Initialize context with graph attributes
	e.mirrorGraphAttributes()

	// Create logs directory
	logsRoot := filepath.Join(e.Config.LogsRoot, e.runID)
	if err := os.MkdirAll(logsRoot, 0755); err != nil {
		return pcontext.Outcome{}, fmt.Errorf("create logs dir: %w", err)
	}

	// Find start node
	startNode := e.Graph.FindStartNode()
	if startNode == nil {
		return pcontext.Outcome{}, fmt.Errorf("no start node found (shape=Mdiamond)")
	}

	// Find exit node (for validation)
	exitNode := e.Graph.FindExitNode()
	if exitNode == nil {
		return pcontext.Outcome{}, fmt.Errorf("no exit node found (shape=Msquare)")
	}

	e.Events.EmitPipelineStart(e.Graph.Name, e.Graph.Goal())

	// Persist run record
	if e.Store != nil {
		_ = e.Store.CreateRun(store.Run{
			ID:        e.runID,
			Mode:      "pipeline",
			GraphName: e.Graph.Name,
			Goal:      e.Graph.Goal(),
			Model:     e.Model(),
			StartedAt: time.Now(),
		})
	}

	// Wire store into handlers that support persistence
	e.Registry.SetStore(e.Store, e.runID)

	currentNode := startNode
	var lastOutcome pcontext.Outcome
	startTime := time.Now()

	// Resume from checkpoint if configured
	completedSet := make(map[string]bool)
	if e.Config.ResumeFromCheckpoint {
		cp, err := pcontext.LoadCheckpoint(logsRoot)
		if err != nil {
			e.Events.EmitLog("warn", fmt.Sprintf("checkpoint load failed, starting fresh: %v", err))
		} else {
			for _, nodeID := range cp.CompletedNodes {
				completedSet[nodeID] = true
			}
			e.completedNodes = cp.CompletedNodes
			e.nodeOutcomes = cp.NodeOutcomes
			if cp.Context != nil {
				e.Context.Merge(cp.Context)
			}
			e.Events.EmitLog("info", fmt.Sprintf("Resuming from checkpoint with %d completed nodes", len(completedSet)))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return pcontext.Outcome{}, ctx.Err()
		default:
		}

		node := currentNode

		// Skip nodes already completed (checkpoint resume)
		if completedSet[node.ID] {
			e.Events.EmitNodeSkip(node.ID, node.Label(), "already completed in previous run")
			// Use stored outcome for edge selection
			if outcome, ok := e.nodeOutcomes[node.ID]; ok {
				lastOutcome = outcome
				// Apply context updates from stored outcome
				for key, value := range outcome.ContextUpdates {
					e.Context.Set(key, value)
				}
				e.Context.Set("outcome", string(outcome.Status))
				if outcome.PreferredLabel != "" {
					e.Context.Set("preferred_label", outcome.PreferredLabel)
				}
				// Select next edge based on stored outcome
				nextEdge := e.selectEdge(node, outcome)
				if nextEdge == nil {
					if e.Graph.IsTerminal(node.ID) {
						break
					}
					break
				}
				nextNode, exists := e.Graph.Nodes[nextEdge.To]
				if !exists {
					return pcontext.Outcome{}, fmt.Errorf("edge target node not found: %s", nextEdge.To)
				}
				currentNode = nextNode
				continue
			}
			// No stored outcome — fall through to re-execute
		}

		// Check for terminal node
		if e.Graph.IsTerminal(node.ID) {
			// Check goal gates
			ok, failedGate := e.checkGoalGates()
			if !ok && failedGate != nil {
				retryTarget := e.getRetryTarget(failedGate)
				if retryTarget != "" {
					if targetNode, exists := e.Graph.Nodes[retryTarget]; exists {
						currentNode = targetNode
						continue
					}
				}
				return pcontext.Outcome{}, fmt.Errorf("goal gate unsatisfied: %s", failedGate.ID)
			}
			break
		}

		// Execute node
		e.Events.EmitNodeStart(node.ID, node.Label(), e.resolveHandlerType(node))

		outcome := e.executeWithRetry(ctx, node, logsRoot)

		// Record completion
		e.completedNodes = append(e.completedNodes, node.ID)
		e.nodeOutcomes[node.ID] = outcome
		lastOutcome = outcome

		// Apply context updates
		for key, value := range outcome.ContextUpdates {
			e.Context.Set(key, value)
		}
		e.Context.Set("outcome", string(outcome.Status))
		if outcome.PreferredLabel != "" {
			e.Context.Set("preferred_label", outcome.PreferredLabel)
		}

		e.Events.EmitNodeEnd(node.ID, string(outcome.Status), outcome.Notes, outcome.FailureReason)

		// Save checkpoint
		if e.Config.CheckpointAfter {
			cp := pcontext.NewCheckpoint(e.runID, node.ID, e.completedNodes, e.Context)
			cp.NodeOutcomes = e.nodeOutcomes
			if err := cp.Save(logsRoot); err != nil {
				e.Events.EmitLog("warn", fmt.Sprintf("checkpoint save failed: %v", err))
			} else {
				e.Events.EmitCheckpoint(node.ID)
			}
		}

		// Persist context snapshot to DB
		if e.Store != nil {
			_ = e.Store.InsertContextSnapshot(e.runID, node.ID, e.Context.All(), e.completedNodes)
		}

		// Select next edge
		nextEdge := e.selectEdge(node, outcome)
		if nextEdge == nil {
			if outcome.Status == pcontext.StatusFail {
				return outcome, fmt.Errorf("stage failed with no outgoing fail edge: %s", node.ID)
			}
			break
		}

		e.Events.EmitEdgeSelected(node.ID, nextEdge.To, nextEdge.Label())

		// Handle loop_restart
		if nextEdge.LoopRestart() {
			// Restart would require re-running the engine
			e.Events.EmitLog("info", "Loop restart triggered")
			return lastOutcome, nil
		}

		// Advance to next node
		nextNode, exists := e.Graph.Nodes[nextEdge.To]
		if !exists {
			return pcontext.Outcome{}, fmt.Errorf("edge target node not found: %s", nextEdge.To)
		}
		currentNode = nextNode
	}

	duration := time.Since(startTime)
	e.Events.EmitPipelineEnd(string(lastOutcome.Status), duration.String())

	// Persist run completion
	if e.Store != nil {
		total, _, _ := e.totalUsage.Cost(e.Model())
		_ = e.Store.UpdateRun(e.runID, store.RunUpdate{
			Status:            string(lastOutcome.Status),
			EndedAt:           time.Now(),
			DurationMs:        duration.Milliseconds(),
			TotalInputTokens:  e.totalUsage.InputTokens,
			TotalOutputTokens: e.totalUsage.OutputTokens,
			TotalCostUSD:      total,
		})
	}

	return lastOutcome, nil
}

func (e *Engine) mirrorGraphAttributes() {
	for key, value := range e.Graph.Attributes {
		e.Context.Set("graph."+key, value)
	}
	if goal := e.Graph.Goal(); goal != "" {
		e.Context.Set("goal", goal)
	}
}

func (e *Engine) resolveHandlerType(node *dot.Node) string {
	if node.Type() != "" {
		return node.Type()
	}
	switch node.Shape() {
	case "Mdiamond":
		return "start"
	case "Msquare":
		return "exit"
	case "hexagon":
		return "wait.human"
	case "diamond":
		return "conditional"
	case "component":
		return "parallel"
	case "tripleoctagon":
		return "parallel.fan_in"
	case "parallelogram":
		return "tool"
	default:
		return "codergen"
	}
}

func (e *Engine) executeWithRetry(ctx context.Context, node *dot.Node, logsRoot string) pcontext.Outcome {
	maxRetries := node.MaxRetries()
	if maxRetries == 0 {
		maxRetries = e.Graph.DefaultMaxRetry()
	}
	maxAttempts := maxRetries + 1

	handler := e.Registry.Resolve(node)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		outcome := handler.Execute(ctx, node, e.Context, e.Graph, logsRoot, e.Events)

		switch outcome.Status {
		case pcontext.StatusSuccess, pcontext.StatusPartialSuccess:
			return outcome
		case pcontext.StatusRetry:
			if attempt < maxAttempts {
				e.Events.EmitNodeRetry(node.ID, attempt, maxAttempts, outcome.FailureReason)
				delay := e.calculateBackoff(attempt)
				time.Sleep(delay)
				continue
			}
			// Retries exhausted
			if node.AllowPartial() {
				return pcontext.Outcome{
					Status: pcontext.StatusPartialSuccess,
					Notes:  "retries exhausted, partial accepted",
				}
			}
			return pcontext.NewFailOutcome("max retries exceeded")
		case pcontext.StatusFail:
			return outcome
		}
	}

	return pcontext.NewFailOutcome("max retries exceeded")
}

func (e *Engine) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: 200ms * 2^(attempt-1), capped at 60s
	base := 200 * time.Millisecond
	delay := base * time.Duration(1<<(attempt-1))
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	return delay
}

func (e *Engine) selectEdge(node *dot.Node, outcome pcontext.Outcome) *dot.Edge {
	edges := e.Graph.OutgoingEdges(node.ID)
	if len(edges) == 0 {
		return nil
	}

	// Step 1: Condition-matching edges
	var conditionMatched []*dot.Edge
	for _, edge := range edges {
		if edge.Condition() != "" {
			met := e.evaluateCondition(edge.Condition(), outcome)
			e.Events.EmitEdgeEvaluated(node.ID, edge.To, edge.Condition(), met)
			if met {
				conditionMatched = append(conditionMatched, edge)
			}
		}
	}
	if len(conditionMatched) > 0 {
		return e.bestByWeightThenLexical(conditionMatched)
	}

	// Step 2: Preferred label match
	if outcome.PreferredLabel != "" {
		normalizedPref := normalizeLabel(outcome.PreferredLabel)
		for _, edge := range edges {
			if normalizeLabel(edge.Label()) == normalizedPref {
				return edge
			}
		}
	}

	// Step 3: Suggested next IDs
	if len(outcome.SuggestedNexts) > 0 {
		for _, suggestedID := range outcome.SuggestedNexts {
			for _, edge := range edges {
				if edge.To == suggestedID {
					return edge
				}
			}
		}
	}

	// Step 4 & 5: Weight with lexical tiebreak (unconditional edges only)
	var unconditional []*dot.Edge
	for _, edge := range edges {
		if edge.Condition() == "" {
			unconditional = append(unconditional, edge)
		}
	}
	if len(unconditional) > 0 {
		return e.bestByWeightThenLexical(unconditional)
	}

	// Fallback: any edge
	return e.bestByWeightThenLexical(edges)
}

func (e *Engine) bestByWeightThenLexical(edges []*dot.Edge) *dot.Edge {
	if len(edges) == 0 {
		return nil
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Weight() != edges[j].Weight() {
			return edges[i].Weight() > edges[j].Weight() // Descending
		}
		return edges[i].To < edges[j].To // Ascending lexical
	})

	return edges[0]
}

func (e *Engine) evaluateCondition(condition string, outcome pcontext.Outcome) bool {
	// Simple condition evaluation
	// Supports: outcome=success, outcome!=fail, key=value, key!=value

	condition = strings.TrimSpace(condition)

	// Parse operator and operands
	var key, op, value string
	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) != 2 {
			return false
		}
		key, op, value = strings.TrimSpace(parts[0]), "!=", strings.TrimSpace(parts[1])
	} else if strings.Contains(condition, "=") {
		parts := strings.SplitN(condition, "=", 2)
		if len(parts) != 2 {
			return false
		}
		key, op, value = strings.TrimSpace(parts[0]), "=", strings.TrimSpace(parts[1])
	} else {
		// Assume truthiness check
		key = condition
		op = "="
		value = "true"
	}

	// Get actual value
	var actualValue string
	switch key {
	case "outcome":
		actualValue = string(outcome.Status)
	case "status":
		actualValue = string(outcome.Status)
	default:
		v, ok := e.Context.Get(key)
		if !ok {
			actualValue = ""
		} else {
			actualValue = fmt.Sprintf("%v", v)
		}
	}

	// Evaluate
	switch op {
	case "=":
		return strings.EqualFold(actualValue, value)
	case "!=":
		return !strings.EqualFold(actualValue, value)
	default:
		return false
	}
}

func (e *Engine) checkGoalGates() (bool, *dot.Node) {
	for _, nodeID := range e.completedNodes {
		node := e.Graph.Nodes[nodeID]
		if node != nil && node.GoalGate() {
			outcome, exists := e.nodeOutcomes[nodeID]
			e.Events.EmitGoalGateCheck(nodeID, string(outcome.Status))
			if exists && outcome.Status != pcontext.StatusSuccess && outcome.Status != pcontext.StatusPartialSuccess {
				e.Events.EmitGoalGateFail(nodeID, outcome.FailureReason)
				return false, node
			}
		}
	}
	return true, nil
}

func (e *Engine) getRetryTarget(node *dot.Node) string {
	// Node-level retry target
	if target := node.RetryTarget(); target != "" {
		return target
	}
	// Node-level fallback
	if target := node.FallbackRetryTarget(); target != "" {
		return target
	}
	// Graph-level retry target
	if target := e.Graph.RetryTarget(); target != "" {
		return target
	}
	// Graph-level fallback
	return e.Graph.FallbackRetryTarget()
}

func normalizeLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))

	// Strip accelerator prefixes
	// [X] prefix
	re1 := regexp.MustCompile(`^\[[a-z]\]\s*`)
	label = re1.ReplaceAllString(label, "")

	// X) prefix
	re2 := regexp.MustCompile(`^[a-z]\)\s*`)
	label = re2.ReplaceAllString(label, "")

	// X - prefix
	re3 := regexp.MustCompile(`^[a-z]\s*-\s*`)
	label = re3.ReplaceAllString(label, "")

	return label
}

// Subscribe returns the event stream for this engine.
func (e *Engine) Subscribe() <-chan events.Event {
	return e.Events.Subscribe()
}

// TotalUsage returns cumulative token usage across all nodes.
func (e *Engine) TotalUsage() llm.Usage {
	return e.totalUsage
}

// Model returns the default model used by this engine.
func (e *Engine) Model() string {
	// Use graph-level model if set
	if model := e.Graph.Attributes["llm_model"]; model != "" {
		return model
	}
	return "minimax/minimax-m2.5"
}

// Close cleans up engine resources.
func (e *Engine) Close() {
	e.Events.Close()
}

// RunID returns the unique identifier for this engine's run.
func (e *Engine) RunID() string {
	return e.runID
}
