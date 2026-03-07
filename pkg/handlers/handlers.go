// Package handlers provides node handlers for the pipeline execution engine.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rhystic/attractor/pkg/agent"
	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/llm"
	"github.com/rhystic/attractor/pkg/store"
)

// Handler executes a node in the pipeline.
type Handler interface {
	// Execute runs the node and returns an outcome.
	Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome
}

// Registry maps handler types to implementations.
type Registry struct {
	handlers       map[string]Handler
	defaultHandler Handler
	codergen       *CodergenHandler // reference kept for store wiring
}

// NewRegistry creates a new handler registry with defaults.
func NewRegistry(llmClient *llm.Client) *Registry {
	r := &Registry{
		handlers: make(map[string]Handler),
	}

	// Register built-in handlers
	r.Register("start", &StartHandler{})
	r.Register("exit", &ExitHandler{})
	r.Register("conditional", &ConditionalHandler{})

	// Codergen handler is the default
	codergen := &CodergenHandler{Client: llmClient}
	r.Register("codergen", codergen)
	r.defaultHandler = codergen
	r.codergen = codergen

	r.Register("wait.human", &WaitForHumanHandler{})
	r.Register("parallel", &ParallelHandler{})
	r.Register("parallel.fan_in", &FanInHandler{})
	r.Register("tool", &ToolHandler{})

	return r
}

// Register adds a handler for a type.
func (r *Registry) Register(typ string, handler Handler) {
	r.handlers[typ] = handler
}

// SetStore configures the persistence store and run ID on handlers that
// support it. This allows the engine to wire persistence after construction.
func (r *Registry) SetStore(s *store.Store, runID string) {
	if r.codergen != nil {
		r.codergen.Store = s
		r.codergen.RunID = runID
	}
}

// Resolve returns the handler for a node.
func (r *Registry) Resolve(node *dot.Node) Handler {
	// Check explicit type first
	if node.Type() != "" {
		if h, ok := r.handlers[node.Type()]; ok {
			return h
		}
	}

	// Shape-based resolution
	handlerType := shapeToHandler(node.Shape())
	if h, ok := r.handlers[handlerType]; ok {
		return h
	}

	return r.defaultHandler
}

func shapeToHandler(shape string) string {
	switch shape {
	case "Mdiamond":
		return "start"
	case "Msquare":
		return "exit"
	case "box":
		return "codergen"
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
	case "house":
		return "stack.manager_loop"
	default:
		return "codergen"
	}
}

// StartHandler is a no-op handler for the pipeline entry point.
type StartHandler struct{}

func (h *StartHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	return pcontext.NewSuccessOutcome("Pipeline started")
}

// ExitHandler is a no-op handler for the pipeline exit point.
type ExitHandler struct{}

func (h *ExitHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	return pcontext.NewSuccessOutcome("Pipeline completed")
}

// ConditionalHandler handles diamond-shaped conditional nodes.
type ConditionalHandler struct{}

func (h *ConditionalHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	return pcontext.NewSuccessOutcome("Conditional node evaluated: " + node.ID)
}

// CodergenHandler executes LLM-based code generation tasks.
type CodergenHandler struct {
	Client *llm.Client
	Store  *store.Store // Optional persistence; nil disables DB writes.
	RunID  string       // Set by engine before execution.
}

func (h *CodergenHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	// Build prompt
	prompt := node.Prompt()
	if prompt == "" {
		prompt = node.Label()
	}

	// Expand $goal variable
	if graph.Goal() != "" {
		prompt = strings.ReplaceAll(prompt, "$goal", graph.Goal())
	}

	// Create stage directory
	stageDir := filepath.Join(logsRoot, node.ID)
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		return pcontext.NewFailOutcome(fmt.Sprintf("create stage dir: %v", err))
	}

	// Write prompt
	if err := os.WriteFile(filepath.Join(stageDir, "prompt.md"), []byte(prompt), 0644); err != nil {
		return pcontext.NewFailOutcome(fmt.Sprintf("write prompt: %v", err))
	}

	emitter.EmitLLMStart(node.ID, node.LLMModel(), prompt)

	if h.Client == nil {
		// Simulation mode
		response := fmt.Sprintf("[Simulated] Response for stage: %s", node.ID)
		if err := os.WriteFile(filepath.Join(stageDir, "response.md"), []byte(response), 0644); err != nil {
			return pcontext.NewFailOutcome(fmt.Sprintf("write response: %v", err))
		}
		return pcontext.NewSuccessOutcome("Stage completed (simulated): "+node.ID).
			WithContextUpdate("last_stage", node.ID).
			WithContextUpdate("last_response", truncate(response, 200))
	}

	// Run agent
	cfg := agent.DefaultConfig()
	if model := node.LLMModel(); model != "" {
		cfg.Model = model
	}
	if effort := node.ReasoningEffort(); effort != "" {
		cfg.ReasoningEffort = effort
	}

	session := agent.NewSession(h.Client, cfg)

	// Subscribe to events
	eventCh := session.Events.Subscribe()
	go func() {
		for event := range eventCh {
			// Forward events with node ID
			event.NodeID = node.ID
			emitter.Emit(event)
		}
	}()

	if err := session.Submit(ctx, prompt); err != nil {
		emitter.EmitLLMError(node.ID, err)
		return pcontext.NewFailOutcome(fmt.Sprintf("agent error: %v", err))
	}

	response := session.LastResponse()

	// Write response
	if err := os.WriteFile(filepath.Join(stageDir, "response.md"), []byte(response), 0644); err != nil {
		return pcontext.NewFailOutcome(fmt.Sprintf("write response: %v", err))
	}

	emitter.EmitLLMEnd(node.ID, truncate(response, 500),
		session.TotalUsage().InputTokens, session.TotalUsage().OutputTokens)

	// Persist artifacts and conversation history to store
	if h.Store != nil && h.RunID != "" {
		_ = h.Store.InsertArtifact(h.RunID, node.ID, "prompt", prompt)
		_ = h.Store.InsertArtifact(h.RunID, node.ID, "response", response)

		for i, turn := range session.History {
			ct := store.ConversationTurn{
				RunID:     h.RunID,
				NodeID:    node.ID,
				TurnIndex: i,
				Role:      string(turn.Role),
				Content:   turn.Content,
				Timestamp: turn.Timestamp,
			}
			if len(turn.ToolCalls) > 0 {
				if b, err := json.Marshal(turn.ToolCalls); err == nil {
					ct.ToolCalls = string(b)
				}
			}
			if len(turn.Results) > 0 {
				if b, err := json.Marshal(turn.Results); err == nil {
					ct.ToolResults = string(b)
				}
			}
			_ = h.Store.InsertConversationTurn(ct)
		}
	}

	outcome := pcontext.NewSuccessOutcome("Stage completed: "+node.ID).
		WithContextUpdate("last_stage", node.ID).
		WithContextUpdate("last_response", truncate(response, 200))

	pcontext.WriteStatus(stageDir, outcome)

	return outcome
}

// WaitForHumanHandler blocks until a human selects an option.
type WaitForHumanHandler struct {
	// AnswerFunc is called to get human input. If nil, uses default behavior.
	AnswerFunc func(question string, options []string) (string, error)
}

func (h *WaitForHumanHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	// Get outgoing edges to derive choices
	edges := graph.OutgoingEdges(node.ID)
	if len(edges) == 0 {
		return pcontext.NewFailOutcome("No outgoing edges for human gate")
	}

	var options []string
	edgeMap := make(map[string]*dot.Edge)

	for _, edge := range edges {
		label := edge.Label()
		if label == "" {
			label = edge.To
		}
		options = append(options, label)
		edgeMap[normalizeLabel(label)] = edge
	}

	question := node.Label()
	if question == "" {
		question = "Select an option:"
	}

	emitter.EmitHumanWaiting(node.ID, question, options)

	// Get answer, with optional timeout from node configuration
	var selected string
	if h.AnswerFunc != nil {
		timeout := node.Timeout()
		if timeout > 0 {
			type answerResult struct {
				answer string
				err    error
			}
			ch := make(chan answerResult, 1)
			go func() {
				answer, err := h.AnswerFunc(question, options)
				ch <- answerResult{answer, err}
			}()
			select {
			case res := <-ch:
				if res.err != nil {
					return pcontext.NewFailOutcome(fmt.Sprintf("human input error: %v", res.err))
				}
				selected = res.answer
			case <-time.After(timeout):
				selected = options[0]
				emitter.EmitHumanTimeout(node.ID, question, selected)
			}
		} else {
			answer, err := h.AnswerFunc(question, options)
			if err != nil {
				return pcontext.NewFailOutcome(fmt.Sprintf("human input error: %v", err))
			}
			selected = answer
		}
	} else {
		// Default: use first option
		selected = options[0]
	}

	emitter.EmitHumanResponse(node.ID, selected)

	// Find matching edge
	normalized := normalizeLabel(selected)
	if edge, ok := edgeMap[normalized]; ok {
		return pcontext.NewSuccessOutcome("Human selected: "+selected).
			WithContextUpdate("human.gate.selected", selected).
			WithSuggestedNexts(edge.To)
	}

	// Fallback to first option
	return pcontext.NewSuccessOutcome("Human selected: "+selected).
		WithContextUpdate("human.gate.selected", selected).
		WithSuggestedNexts(edges[0].To)
}

// ParallelHandler executes multiple branches concurrently.
type ParallelHandler struct{}

func (h *ParallelHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	edges := graph.OutgoingEdges(node.ID)
	if len(edges) == 0 {
		return pcontext.NewFailOutcome("No branches for parallel execution")
	}

	// Note: Full parallel execution would require executing subgraphs
	// For now, just mark as success and let engine handle routing
	return pcontext.NewSuccessOutcome(fmt.Sprintf("Parallel node with %d branches", len(edges)))
}

// FanInHandler consolidates results from parallel branches.
type FanInHandler struct{}

func (h *FanInHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	// Read parallel results from context
	_, ok := pctx.Get("parallel.results")
	if !ok {
		// No parallel results, just pass through
		return pcontext.NewSuccessOutcome("Fan-in completed (no parallel results)")
	}

	return pcontext.NewSuccessOutcome("Fan-in completed")
}

// ToolHandler executes external tools.
type ToolHandler struct{}

func (h *ToolHandler) Execute(ctx context.Context, node *dot.Node, pctx *pcontext.Context, graph *dot.Graph, logsRoot string, emitter *events.Emitter) pcontext.Outcome {
	command := node.Attributes["tool_command"]
	if command == "" {
		return pcontext.NewFailOutcome("No tool_command specified")
	}

	emitter.EmitToolStart(node.ID, "shell", command)

	// Execute the command
	// This would use the execution environment
	// For now, simulate success
	result := fmt.Sprintf("Tool executed: %s", command)

	emitter.EmitToolOutput(node.ID, "shell", result)
	emitter.EmitToolEnd(node.ID, "shell", result, false)

	return pcontext.NewSuccessOutcome(result).
		WithContextUpdate("tool.output", result)
}

func normalizeLabel(label string) string {
	// Lowercase, trim, strip accelerator prefixes
	label = strings.ToLower(strings.TrimSpace(label))

	// Strip [X] prefix
	if len(label) > 3 && label[0] == '[' && label[2] == ']' {
		label = strings.TrimSpace(label[3:])
	}

	// Strip X) prefix
	if len(label) > 2 && label[1] == ')' {
		label = strings.TrimSpace(label[2:])
	}

	// Strip X - prefix
	if len(label) > 3 && label[1] == ' ' && label[2] == '-' {
		label = strings.TrimSpace(label[3:])
	}

	return label
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
