// Package events provides the event system for pipeline execution.
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType identifies the type of pipeline event.
type EventType string

const (
	// Pipeline lifecycle events
	EventPipelineStart EventType = "pipeline_start"
	EventPipelineEnd   EventType = "pipeline_end"
	EventPipelineError EventType = "pipeline_error"

	// Node lifecycle events
	EventNodeStart EventType = "node_start"
	EventNodeEnd   EventType = "node_end"
	EventNodeRetry EventType = "node_retry"
	EventNodeSkip  EventType = "node_skip"

	// Edge events
	EventEdgeSelected  EventType = "edge_selected"
	EventEdgeEvaluated EventType = "edge_evaluated"

	// LLM events
	EventLLMStart EventType = "llm_start"
	EventLLMDelta EventType = "llm_delta"
	EventLLMEnd   EventType = "llm_end"
	EventLLMError EventType = "llm_error"

	// Tool events
	EventToolStart  EventType = "tool_start"
	EventToolOutput EventType = "tool_output"
	EventToolEnd    EventType = "tool_end"
	EventToolError  EventType = "tool_error"

	// Human interaction events
	EventHumanWaiting  EventType = "human_waiting"
	EventHumanResponse EventType = "human_response"
	EventHumanTimeout  EventType = "human_timeout"

	// Checkpoint events
	EventCheckpoint EventType = "checkpoint"

	// Loop detection
	EventLoopDetected EventType = "loop_detected"

	// Goal gate events
	EventGoalGateCheck EventType = "goal_gate_check"
	EventGoalGateFail  EventType = "goal_gate_fail"

	// Logging events
	EventLog EventType = "log"
)

// Event represents a pipeline execution event.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id,omitempty"`
	NodeID    string    `json:"node_id,omitempty"`
	Data      EventData `json:"data,omitempty"`
}

// EventData holds the payload for an event.
type EventData struct {
	// Pipeline data
	GraphName   string `json:"graph_name,omitempty"`
	Goal        string `json:"goal,omitempty"`
	FinalStatus string `json:"final_status,omitempty"`
	Duration    string `json:"duration,omitempty"`

	// Node data
	NodeLabel     string `json:"node_label,omitempty"`
	NodeType      string `json:"node_type,omitempty"`
	Status        string `json:"status,omitempty"`
	Notes         string `json:"notes,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	AttemptNum    int    `json:"attempt_num,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`

	// Edge data
	EdgeLabel    string `json:"edge_label,omitempty"`
	FromNode     string `json:"from_node,omitempty"`
	ToNode       string `json:"to_node,omitempty"`
	Condition    string `json:"condition,omitempty"`
	ConditionMet bool   `json:"condition_met,omitempty"`

	// LLM data
	Model        string `json:"model,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	Delta        string `json:"delta,omitempty"`
	Response     string `json:"response,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`

	// Tool data
	ToolName   string `json:"tool_name,omitempty"`
	ToolArgs   string `json:"tool_args,omitempty"`
	ToolOutput string `json:"tool_output,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`

	// Human data
	Question string   `json:"question,omitempty"`
	Options  []string `json:"options,omitempty"`
	Selected string   `json:"selected,omitempty"`

	// Log data
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`

	// Error data
	Error string `json:"error,omitempty"`

	// Raw data for extensibility
	Raw map[string]any `json:"raw,omitempty"`
}

// String returns a JSON representation of the event.
func (e Event) String() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// Emitter emits events to subscribers.
type Emitter struct {
	mu          sync.RWMutex
	subscribers []chan Event
	runID       string
}

// NewEmitter creates a new event emitter.
func NewEmitter(runID string) *Emitter {
	return &Emitter{
		runID: runID,
	}
}

// Subscribe adds a subscriber channel.
func (e *Emitter) Subscribe() <-chan Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan Event, 100)
	e.subscribers = append(e.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (e *Emitter) Unsubscribe(ch <-chan Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, sub := range e.subscribers {
		if sub == ch {
			e.subscribers = append(e.subscribers[:i], e.subscribers[i+1:]...)
			close(sub)
			return
		}
	}
}

// Emit sends an event to all subscribers.
func (e *Emitter) Emit(event Event) {
	event.Timestamp = time.Now()
	event.RunID = e.runID

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ch := range e.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip (non-blocking)
		}
	}
}

// EmitNodeStart emits a node start event.
func (e *Emitter) EmitNodeStart(nodeID, nodeLabel, nodeType string) {
	e.Emit(Event{
		Type:   EventNodeStart,
		NodeID: nodeID,
		Data: EventData{
			NodeLabel: nodeLabel,
			NodeType:  nodeType,
		},
	})
}

// EmitNodeEnd emits a node end event.
func (e *Emitter) EmitNodeEnd(nodeID string, status, notes, failureReason string) {
	e.Emit(Event{
		Type:   EventNodeEnd,
		NodeID: nodeID,
		Data: EventData{
			Status:        status,
			Notes:         notes,
			FailureReason: failureReason,
		},
	})
}

// EmitNodeRetry emits a node retry event.
func (e *Emitter) EmitNodeRetry(nodeID string, attemptNum, maxAttempts int, reason string) {
	e.Emit(Event{
		Type:   EventNodeRetry,
		NodeID: nodeID,
		Data: EventData{
			AttemptNum:    attemptNum,
			MaxAttempts:   maxAttempts,
			FailureReason: reason,
		},
	})
}

// EmitEdgeSelected emits an edge selected event.
func (e *Emitter) EmitEdgeSelected(fromNode, toNode, label string) {
	e.Emit(Event{
		Type:   EventEdgeSelected,
		NodeID: fromNode,
		Data: EventData{
			FromNode:  fromNode,
			ToNode:    toNode,
			EdgeLabel: label,
		},
	})
}

// EmitLLMStart emits an LLM start event.
func (e *Emitter) EmitLLMStart(nodeID, model, prompt string) {
	e.Emit(Event{
		Type:   EventLLMStart,
		NodeID: nodeID,
		Data: EventData{
			Model:  model,
			Prompt: prompt,
		},
	})
}

// EmitLLMDelta emits an LLM streaming delta event.
func (e *Emitter) EmitLLMDelta(nodeID, delta string) {
	e.Emit(Event{
		Type:   EventLLMDelta,
		NodeID: nodeID,
		Data: EventData{
			Delta: delta,
		},
	})
}

// EmitLLMEnd emits an LLM end event.
func (e *Emitter) EmitLLMEnd(nodeID, response string, inputTokens, outputTokens int) {
	e.Emit(Event{
		Type:   EventLLMEnd,
		NodeID: nodeID,
		Data: EventData{
			Response:     response,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	})
}

// EmitToolStart emits a tool start event.
func (e *Emitter) EmitToolStart(nodeID, toolName, toolArgs string) {
	e.Emit(Event{
		Type:   EventToolStart,
		NodeID: nodeID,
		Data: EventData{
			ToolName: toolName,
			ToolArgs: toolArgs,
		},
	})
}

// EmitToolEnd emits a tool end event.
func (e *Emitter) EmitToolEnd(nodeID, toolName, output string, isError bool) {
	e.Emit(Event{
		Type:   EventToolEnd,
		NodeID: nodeID,
		Data: EventData{
			ToolName:   toolName,
			ToolOutput: output,
			IsError:    isError,
		},
	})
}

// EmitHumanWaiting emits a human waiting event.
func (e *Emitter) EmitHumanWaiting(nodeID, question string, options []string) {
	e.Emit(Event{
		Type:   EventHumanWaiting,
		NodeID: nodeID,
		Data: EventData{
			Question: question,
			Options:  options,
		},
	})
}

// EmitHumanResponse emits a human response event.
func (e *Emitter) EmitHumanResponse(nodeID, selected string) {
	e.Emit(Event{
		Type:   EventHumanResponse,
		NodeID: nodeID,
		Data: EventData{
			Selected: selected,
		},
	})
}

// EmitLog emits a log event.
func (e *Emitter) EmitLog(level, message string) {
	e.Emit(Event{
		Type: EventLog,
		Data: EventData{
			Level:   level,
			Message: message,
		},
	})
}

// EmitError emits an error event.
func (e *Emitter) EmitError(nodeID string, err error) {
	e.Emit(Event{
		Type:   EventPipelineError,
		NodeID: nodeID,
		Data: EventData{
			Error: err.Error(),
		},
	})
}

// EmitPipelineStart emits a pipeline start event.
func (e *Emitter) EmitPipelineStart(graphName, goal string) {
	e.Emit(Event{
		Type: EventPipelineStart,
		Data: EventData{
			GraphName: graphName,
			Goal:      goal,
		},
	})
}

// EmitPipelineEnd emits a pipeline end event.
func (e *Emitter) EmitPipelineEnd(finalStatus, duration string) {
	e.Emit(Event{
		Type: EventPipelineEnd,
		Data: EventData{
			FinalStatus: finalStatus,
			Duration:    duration,
		},
	})
}

// EmitNodeSkip emits a node skip event (e.g. during checkpoint resume).
func (e *Emitter) EmitNodeSkip(nodeID, nodeLabel, reason string) {
	e.Emit(Event{
		Type:   EventNodeSkip,
		NodeID: nodeID,
		Data: EventData{
			NodeLabel: nodeLabel,
			Notes:     reason,
		},
	})
}

// EmitEdgeEvaluated emits an edge condition evaluation event.
func (e *Emitter) EmitEdgeEvaluated(fromNode, toNode, condition string, conditionMet bool) {
	e.Emit(Event{
		Type:   EventEdgeEvaluated,
		NodeID: fromNode,
		Data: EventData{
			FromNode:     fromNode,
			ToNode:       toNode,
			Condition:    condition,
			ConditionMet: conditionMet,
		},
	})
}

// EmitLLMError emits an LLM error event.
func (e *Emitter) EmitLLMError(nodeID string, err error) {
	e.Emit(Event{
		Type:   EventLLMError,
		NodeID: nodeID,
		Data: EventData{
			Error: err.Error(),
		},
	})
}

// EmitToolOutput emits a tool output event (raw output before tool end).
func (e *Emitter) EmitToolOutput(nodeID, toolName, output string) {
	e.Emit(Event{
		Type:   EventToolOutput,
		NodeID: nodeID,
		Data: EventData{
			ToolName:   toolName,
			ToolOutput: output,
		},
	})
}

// EmitToolError emits a tool error event.
func (e *Emitter) EmitToolError(nodeID, toolName string, err error) {
	e.Emit(Event{
		Type:   EventToolError,
		NodeID: nodeID,
		Data: EventData{
			ToolName: toolName,
			Error:    err.Error(),
		},
	})
}

// EmitHumanTimeout emits a human timeout event.
func (e *Emitter) EmitHumanTimeout(nodeID, question, fallback string) {
	e.Emit(Event{
		Type:   EventHumanTimeout,
		NodeID: nodeID,
		Data: EventData{
			Question: question,
			Selected: fallback,
		},
	})
}

// EmitCheckpoint emits a checkpoint saved event.
func (e *Emitter) EmitCheckpoint(nodeID string) {
	e.Emit(Event{
		Type:   EventCheckpoint,
		NodeID: nodeID,
	})
}

// EmitLoopDetected emits a loop detection event.
func (e *Emitter) EmitLoopDetected(nodeID, message string) {
	e.Emit(Event{
		Type:   EventLoopDetected,
		NodeID: nodeID,
		Data: EventData{
			Message: message,
		},
	})
}

// EmitGoalGateCheck emits a goal gate check event.
func (e *Emitter) EmitGoalGateCheck(nodeID string, status string) {
	e.Emit(Event{
		Type:   EventGoalGateCheck,
		NodeID: nodeID,
		Data: EventData{
			Status: status,
		},
	})
}

// EmitGoalGateFail emits a goal gate failure event.
func (e *Emitter) EmitGoalGateFail(nodeID, failureReason string) {
	e.Emit(Event{
		Type:   EventGoalGateFail,
		NodeID: nodeID,
		Data: EventData{
			FailureReason: failureReason,
		},
	})
}

// Close closes all subscriber channels.
func (e *Emitter) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, ch := range e.subscribers {
		close(ch)
	}
	e.subscribers = nil
}
